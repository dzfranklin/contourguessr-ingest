package flickrindexer

import (
	"context"
	"contourguessr-ingest/flickr"
	"contourguessr-ingest/repos"
	"encoding/json"
	"fmt"
	"github.com/minio/minio-go/v7"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/planar"
	"io"
	"log"
	"os"
	"strconv"
	"time"
)

var maxConcurrentPhotoDownloads = 10
var bucketName = "contourguessr-photos"

type Flickr interface {
	Call(ctx context.Context, method string, out any, params map[string]string) error
	Download(ctx context.Context, url string) (io.ReadCloser, error)
}

type MinIO interface {
	FPutObject(ctx context.Context, bucketName, objectName string, filePath string, opts minio.PutObjectOptions) (info minio.UploadInfo, err error)
}

const verboseContextKey = "verbose"

func NewVerboseContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, verboseContextKey, true)
}

func verboseStatus(ctx context.Context) bool {
	v, _ := ctx.Value(verboseContextKey).(bool)
	return v
}

func Step(
	ctx context.Context,
	fc Flickr,
	mc MinIO,
	region repos.Region,
	cursor repos.Cursor,
) (repos.Cursor, []repos.FlickrPhoto, error) {
	isVerbose := verboseStatus(ctx)

	if cursor.Page == 0 {
		cursor.Page = 1
	}
	if cursor.MinUploadDate.IsZero() {
		cursor.MinUploadDate = time.Unix(0, 0)
	}

	bounds := region.Geo.Bound()
	var searchPage searchResponse
	if isVerbose {
		log.Printf("search: %s from %s page %d", region.Name, cursor.MinUploadDate, cursor.Page)
	}
	err := fc.Call(ctx, "flickr.photos.search", &searchPage, map[string]string{
		"bbox":            fmt.Sprintf("%.6f,%.6f,%.6f,%.6f", bounds.Left(), bounds.Bottom(), bounds.Right(), bounds.Top()),
		"min_upload_date": fmt.Sprintf("%d", cursor.MinUploadDate.Unix()),
		"sort":            "date-posted-asc",
		"safe_search":     "1",
		"content_type":    "1",
		"license":         "1,2,3,4,5,6,7,8,9,10",
		"extras":          "geo,date_upload",
		"per_page":        "100",
		"page":            fmt.Sprintf("%d", cursor.Page),
	})
	if err != nil {
		return repos.Cursor{}, nil, err
	}

	var photos []repos.FlickrPhoto
	for _, sp := range searchPage.Photos.Photo {
		if ctx.Err() != nil {
			return repos.Cursor{}, nil, ctx.Err()
		}

		lng, err := strconv.ParseFloat(sp.Longitude, 64)
		if err != nil {
			return repos.Cursor{}, nil, fmt.Errorf("parse longitude: %w", err)
		}
		lat, err := strconv.ParseFloat(sp.Latitude, 64)
		if err != nil {
			return repos.Cursor{}, nil, fmt.Errorf("parse latitude: %w", err)
		}
		if !planar.MultiPolygonContains(region.Geo, orb.Point{lng, lat}) {
			continue
		}

		p, err := processPhoto(ctx, fc, region.Id, sp)
		if err != nil {
			log.Printf("error processing photo https://flickr.com/photos/%s/%s: %v", sp.Owner, sp.Id, err)
			continue
		}
		photos = append(photos, p)
	}

	err = downloadSizes(ctx, fc, mc, photos)
	if err != nil {
		return repos.Cursor{}, nil, fmt.Errorf("download sizes: %w", err)
	}

	if len(searchPage.Photos.Photo) >= 100 {
		cursor.Page++

		if cursor.Page*100 > 2000 {
			last := searchPage.Photos.Photo[len(searchPage.Photos.Photo)-1]
			lastTime, err := flickr.ParseTime(last.DateUpload)
			if err != nil {
				return repos.Cursor{}, nil, fmt.Errorf("parse dateupload of last photo: %w", err)
			}
			cursor.MinUploadDate = lastTime
			cursor.Page = 1
		}
	} else {
		now := time.Now()
		cursor.LastCheck = &now
	}

	return cursor, photos, nil
}

type searchResponse struct {
	Photos struct {
		Photo []searchResponsePhoto
	}
}

type searchResponsePhoto struct {
	Id         string
	Owner      string
	Latitude   string
	Longitude  string
	DateUpload string
}

func processPhoto(
	ctx context.Context,
	fc Flickr,
	regionId int,
	searchInfo searchResponsePhoto,
) (repos.FlickrPhoto, error) {
	isVerbose := verboseStatus(ctx)
	id := searchInfo.Id
	out := repos.FlickrPhoto{
		Id:       id,
		RegionId: regionId,
	}

	var parsedInfo struct {
		Photo map[string]json.RawMessage
	}
	if isVerbose {
		log.Printf("getInfo: %s", id)
	}
	err := fc.Call(ctx, "flickr.photos.getInfo", &parsedInfo, map[string]string{
		"photo_id": id,
		"extras":   "sizes",
	})
	if err != nil {
		return repos.FlickrPhoto{}, fmt.Errorf("getInfo: %w", err)
	}

	sizesJSON, ok := parsedInfo.Photo["sizes"]
	if !ok {
		return repos.FlickrPhoto{}, fmt.Errorf("gitInfo missing sizes extra")
	}
	var sizes struct {
		Size json.RawMessage
	}
	err = json.Unmarshal(sizesJSON, &sizes)
	if err != nil {
		return repos.FlickrPhoto{}, fmt.Errorf("unmarshal sizes from getInfo extra: %w", err)
	}
	out.Sizes = sizes.Size

	delete(parsedInfo.Photo, "sizes")
	out.Info, err = json.Marshal(parsedInfo.Photo)
	if err != nil {
		return repos.FlickrPhoto{}, fmt.Errorf("marshal info: %w", err)
	}

	var exif struct {
		Photo struct {
			Exif json.RawMessage
		}
	}
	if isVerbose {
		log.Printf("getExif: %s", id)
	}
	err = fc.Call(ctx, "flickr.photos.getExif", &exif, map[string]string{
		"photo_id": id,
	})
	if err != nil {
		return repos.FlickrPhoto{}, fmt.Errorf("getExif: %w", err)
	}
	out.Exif = exif.Photo.Exif

	return out, nil
}

type dlResult struct {
	id  string
	err error
}

func (r dlResult) String() string {
	if r.err != nil {
		return fmt.Sprintf("%s: %v", r.id, r.err)
	} else {
		return fmt.Sprintf("%s: success", r.id)
	}
}

func downloadSizes(ctx context.Context, fc Flickr, mc MinIO, photos []repos.FlickrPhoto) error {
	resultChan := make(chan dlResult, len(photos))

	sem := make(chan struct{}, maxConcurrentPhotoDownloads)
	for i := range photos {
		sem <- struct{}{}
		go func(i int) {
			defer func() { <-sem }()
			id := photos[i].Id
			err := downloadSizesForPhoto(ctx, fc, mc, &photos[i])
			resultChan <- dlResult{id, err}
		}(i)
	}

	var errors []dlResult
	for range photos {
		res := <-resultChan
		if res.err != nil {
			errors = append(errors, res)
		}
	}

	if len(errors) > len(photos)/4 {
		return fmt.Errorf("too many errors: %d/%d failed: %s", len(errors), len(photos), errors)
	}
	return nil
}

func downloadSizesForPhoto(ctx context.Context, fc Flickr, mc MinIO, photo *repos.FlickrPhoto) error {
	isVerbose := verboseStatus(ctx)
	if isVerbose {
		log.Printf("downloadSizesForPhoto: %s", photo.Id)
	}

	type sizeData struct {
		Label  string
		Width  int
		Height int
		Source string
		Media  string
	}
	var sizes []sizeData
	err := json.Unmarshal(photo.Sizes, &sizes)
	if err != nil {
		return fmt.Errorf("unmarshal sizes: %w", err)
	}

	var medium *sizeData
	var large *sizeData
	for _, size := range sizes {
		if size.Media != "photo" ||
			size.Label == "Square" || size.Label == "Large Square" || // is cropped
			size.Label == "Original" { // has exif rotation nuances, etc
			continue
		}

		if size.Label == "Medium" {
			medium = &size
		}

		if large == nil || size.Width > large.Width {
			large = &size
		}
	}
	if medium == nil {
		return fmt.Errorf("missing medium size")
	}
	if large == nil {
		return fmt.Errorf("missing large size")
	}

	mediumFile, err := os.CreateTemp("", "")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	largeFile, err := os.CreateTemp("", "")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func(medium, large string) {
		if err := os.Remove(medium); err != nil {
			log.Printf("remove medium temp file: %v", err)
		}
		if err := os.Remove(large); err != nil {
			log.Printf("remove large temp file: %v", err)
		}
	}(mediumFile.Name(), largeFile.Name())

	if isVerbose {
		log.Printf("download: %s medium %s", photo.Id, medium.Source)
	}
	mediumReader, err := fc.Download(ctx, medium.Source)
	if err != nil {
		return fmt.Errorf("download medium: %w", err)
	}
	defer mediumReader.Close()
	_, err = io.Copy(mediumFile, mediumReader)
	if err != nil {
		return fmt.Errorf("download medium: %w", err)
	}

	if isVerbose {
		log.Printf("download: %s large %s", photo.Id, large.Source)
	}
	largeReader, err := fc.Download(ctx, large.Source)
	if err != nil {
		return fmt.Errorf("download large: %w", err)
	}
	defer largeReader.Close()
	_, err = io.Copy(largeFile, largeReader)
	if err != nil {
		return fmt.Errorf("download large: %w", err)
	}

	mediumKey := fmt.Sprintf("flickr/%s/medium.jpg", photo.Id)
	if isVerbose {
		log.Printf("upload: %s", mediumKey)
	}
	_, err = mc.FPutObject(ctx, bucketName, mediumKey, mediumFile.Name(), minio.PutObjectOptions{
		ContentType: "image/jpeg",
	})
	if err != nil {
		return fmt.Errorf("upload medium: %w", err)
	}

	largeKey := fmt.Sprintf("flickr/%s/large.jpg", photo.Id)
	if isVerbose {
		log.Printf("upload: %s", largeKey)
	}
	_, err = mc.FPutObject(ctx, bucketName, largeKey, largeFile.Name(), minio.PutObjectOptions{
		ContentType: "image/jpeg",
	})
	if err != nil {
		return fmt.Errorf("upload large: %w", err)
	}

	photo.Medium = repos.PhotoSize{
		Width:  medium.Width,
		Height: medium.Height,
		Source: "https://minio.dfranklin.dev/contourguessr-photos/" + mediumKey,
	}

	photo.Large = repos.PhotoSize{
		Width:  large.Width,
		Height: large.Height,
		Source: "https://minio.dfranklin.dev/contourguessr-photos/" + largeKey,
	}

	return nil
}

package flickrindexer

import (
	"context"
	"contourguessr-ingest/flickr"
	"contourguessr-ingest/repos"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/minio/minio-go/v7"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/planar"
	"io"
	"log/slog"
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

func Step(
	ctx context.Context,
	fc Flickr,
	mc MinIO,
	region repos.Region,
	cursor repos.Cursor,
) (repos.Cursor, []repos.Photo, error) {
	if cursor.Page == 0 {
		cursor.Page = 1
	}
	if cursor.MinUploadDate.IsZero() {
		cursor.MinUploadDate = time.Unix(0, 0)
	}

	bounds := region.Geo.Bound()
	var searchPage searchResponse
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

	var photos []repos.Photo
	skipped := 0
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
			slog.Debug("skip: not in region", "id", sp.Id, "lng", lng, "lat", lat)
			skipped++
			continue
		}

		p, err := processPhoto(ctx, fc, region.Id, sp)
		if errors.Is(err, ErrorSkipPhoto) {
			skipped++
			continue
		} else if err != nil {
			slog.Error(fmt.Sprintf("error processing photo: %s", err), "owner", sp.Owner, "id", sp.Id)
			skipped++
			continue
		}
		photos = append(photos, p)
	}

	err = downloadSizes(ctx, fc, mc, photos)
	if err != nil {
		return repos.Cursor{}, nil, fmt.Errorf("download sizes: %w", err)
	}

	slog.Info("skipped", "skipped", skipped, "total", len(searchPage.Photos.Photo))

	if searchPage.Photos.Page >= searchPage.Photos.Pages {
		now := time.Now()
		cursor.LastCheck = &now

		cursor.MinUploadDate = time.Now().Add(-1 * time.Hour)
		cursor.Page = 1

		slog.Info("completed round of checking", "region_id", region.Id, "region_name", region.Name)
	} else {
		cursor.Page++
		slog.Info("advancing page", "region_id", region.Id, "region_name", region.Name, "new_value", cursor.Page)

		if cursor.Page > 20 {
			last := searchPage.Photos.Photo[len(searchPage.Photos.Photo)-1]
			lastTime, err := flickr.ParseTime(last.DateUpload)
			if err != nil {
				return repos.Cursor{}, nil, fmt.Errorf("parse dateupload of last photo: %w", err)
			}
			cursor.MinUploadDate = lastTime
			cursor.Page = 1
			slog.Info("advancing min_upload_date", "region_id", region.Id, "region_name", region.Name, "new_value", cursor.MinUploadDate)
		}
	}

	return cursor, photos, nil
}

type searchResponse struct {
	Photos struct {
		Page  int
		Pages int
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

var ErrorSkipPhoto = fmt.Errorf("skip photo")

func processPhoto(
	ctx context.Context,
	fc Flickr,
	regionId int,
	searchInfo searchResponsePhoto,
) (repos.Photo, error) {
	id := searchInfo.Id
	out := repos.Photo{
		Id:       id,
		RegionId: regionId,
	}

	var exif struct {
		Photo struct {
			Exif json.RawMessage
		}
	}
	err := fc.Call(ctx, "flickr.photos.getExif", &exif, map[string]string{
		"photo_id": id,
	})
	if err != nil {
		return repos.Photo{}, fmt.Errorf("getExif: %w", err)
	}
	out.Exif = exif.Photo.Exif

	if exif.Photo.Exif == nil {
		slog.Debug("skip: no exif", "id", id)
		return repos.Photo{}, ErrorSkipPhoto
	}
	var parsedExif []struct {
		Tag string
	}
	err = json.Unmarshal(exif.Photo.Exif, &parsedExif)
	if err != nil {
		return repos.Photo{}, fmt.Errorf("unmarshal exif: %w", err)
	}
	var hasGPSLng, hasGPSLat bool
	for _, tag := range parsedExif {
		switch tag.Tag {
		case "GPSLongitude":
			hasGPSLng = true
		case "GPSLatitude":
			hasGPSLat = true
		}
	}
	if !(hasGPSLng && hasGPSLat) {
		slog.Debug("skip: no gps lng/lat", "id", id)
		return repos.Photo{}, ErrorSkipPhoto
	}

	var parsedInfo struct {
		Photo map[string]json.RawMessage
	}
	err = fc.Call(ctx, "flickr.photos.getInfo", &parsedInfo, map[string]string{
		"photo_id": id,
		"extras":   "sizes",
	})
	if err != nil {
		return repos.Photo{}, fmt.Errorf("getInfo: %w", err)
	}

	sizesJSON, ok := parsedInfo.Photo["sizes"]
	if !ok {
		return repos.Photo{}, fmt.Errorf("gitInfo missing sizes extra")
	}
	var sizes struct {
		Size json.RawMessage
	}
	err = json.Unmarshal(sizesJSON, &sizes)
	if err != nil {
		return repos.Photo{}, fmt.Errorf("unmarshal sizes from getInfo extra: %w", err)
	}
	out.Sizes = sizes.Size

	delete(parsedInfo.Photo, "sizes")
	out.Info, err = json.Marshal(parsedInfo.Photo)
	if err != nil {
		return repos.Photo{}, fmt.Errorf("marshal info: %w", err)
	}

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

func downloadSizes(ctx context.Context, fc Flickr, mc MinIO, photos []repos.Photo) error {
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

	var errs []dlResult
	for range photos {
		res := <-resultChan
		if res.err != nil {
			errs = append(errs, res)
			slog.Error("download failed", "id", res.id, "err", res.err)
		}
	}

	if len(errs) > len(photos)/4 {
		return fmt.Errorf("too many errors: %d/%d failed: %s", len(errs), len(photos), errs)
	}
	return nil
}

func downloadSizesForPhoto(ctx context.Context, fc Flickr, mc MinIO, photo *repos.Photo) error {
	slog.Debug("downloadSizesForPhoto", "id", photo.Id)

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
			(size.Width > 2048 || size.Height > 2048) || // save storage space
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
	// TODO: This can happen for a small original photo. Should propagate up that it's a normal skip.
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
			slog.Error("remove medium temp file", "err", err)
		}
		if err := os.Remove(large); err != nil {
			slog.Error("remove large temp file", "err", err)
		}
	}(mediumFile.Name(), largeFile.Name())

	slog.Debug("download medium", "id", photo.Id, "source", medium.Source)
	mediumReader, err := fc.Download(ctx, medium.Source)
	if err != nil {
		return fmt.Errorf("download medium: %w", err)
	}
	defer mediumReader.Close()
	_, err = io.Copy(mediumFile, mediumReader)
	if err != nil {
		return fmt.Errorf("download medium: %w", err)
	}

	slog.Debug("download large", "id", photo.Id, "source", large.Source)
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
	slog.Debug("upload medium", "id", photo.Id, "key", mediumKey)
	_, err = mc.FPutObject(ctx, bucketName, mediumKey, mediumFile.Name(), minio.PutObjectOptions{
		ContentType: "image/jpeg",
	})
	if err != nil {
		return fmt.Errorf("upload medium: %w", err)
	}

	largeKey := fmt.Sprintf("flickr/%s/large.jpg", photo.Id)
	slog.Debug("upload large", "id", photo.Id, "key", largeKey)
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

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/joho/godotenv"
)

var flickrAPIKey string
var region string
var regionBbox string

func init() {
	err := godotenv.Load(".env.local")
	if err != nil {
		log.Fatal("Error loading .env file", err)
	}

	flickrAPIKey = os.Getenv("FLICKR_API_KEY")
	if flickrAPIKey == "" {
		log.Fatal("FLICKR_API_KEY not set")
	}

	region = os.Getenv("REGION")
	if region == "" {
		log.Fatal("REGION not set")
	}

	regionBbox = os.Getenv("REGION_BBOX")
	if regionBbox == "" {
		log.Fatal("REGION_BBOX not set")
	}
}

func main() {
	outDir := "out"
	if err := os.MkdirAll(outDir+"/"+region, 0750); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(outDir+"/manifests", 0750); err != nil {
		log.Fatal(err)
	}

	var photos []FlickrPhoto

	for page := 1; page <= 40; page++ {
		time.Sleep(1 * time.Second)

		var searchResponse FlickrSearchResponse
		callFlickr("flickr.photos.search", &searchResponse, map[string]string{
			"page": fmt.Sprintf("%d", page),
			"sort": "interestingness-desc",
			"bbox": regionBbox,
		})
		for _, photo := range searchResponse.Photos.Photo {
			photos = append(photos, photo)
		}

		writeGallery(outDir+"/"+region, page, searchResponse.Photos.Photo)
	}

	log.Printf("Found %d photos", len(photos))

	manifestFilename := fmt.Sprintf("%s/manifests/%s.json", outDir, region)
	manifestFile, err := os.Create(manifestFilename)
	if err != nil {
		log.Fatal(err)
	}
	defer manifestFile.Close()

	enc := json.NewEncoder(manifestFile)
	enc.SetIndent("", "  ")
	if err = enc.Encode(photos); err != nil {
		log.Fatal(err)
	}

	log.Printf("Wrote %s", manifestFilename)
}

func writeGallery(outDir string, page int, photos []FlickrPhoto) {
	galleryFilename := fmt.Sprintf("%s/gallery_%d.html", outDir, page)
	galleryFile, err := os.Create(galleryFilename)
	if err != nil {
		log.Fatal(err)
	}
	_, err = galleryFile.WriteString("<html><body>\n")
	defer func() {
		_, err := galleryFile.WriteString("</body></html>\n")
		if err != nil {
			log.Fatal(err)
		}
		err = galleryFile.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()

	_, err = galleryFile.WriteString("<ul>\n")
	if err != nil {
		log.Fatal(err)
	}
	for _, photo := range photos {
		_, err = fmt.Fprintf(galleryFile, "<li><a href=\"%s\">", flickrImageWebURL(photo))
		if err != nil {
			log.Fatal(err)
		}
		_, err = fmt.Fprintf(galleryFile, "<h2>%s</h2>\n", photo.Title)
		_, err = fmt.Fprintf(galleryFile, "<img src=\"%s\">\n", flickrImagePreviewURL(photo))
		if err != nil {
			log.Fatal(err)
		}
		_, err = galleryFile.WriteString("</a></li>")
		if err != nil {
			log.Fatal(err)
		}
	}
	_, err = galleryFile.WriteString("</ul>\n")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Wrote %s", galleryFilename)
}

type FlickrSearchResponse struct {
	Photos struct {
		Page    int           `json:"page"`
		Pages   int           `json:"pages"`
		PerPage int           `json:"perpage"`
		Total   int           `json:"total"`
		Photo   []FlickrPhoto `json:"photo"`
	} `json:"photos"`
}

type FlickrPhoto struct {
	ID     string `json:"id"`
	Owner  string `json:"owner"`
	Secret string `json:"secret"`
	Server string `json:"server"`
	Title  string `json:"title"`
}

func flickrImagePreviewURL(photo FlickrPhoto) string {
	// https://live.staticflickr.com/{server-id}/{id}_{secret}_{size-suffix}.jpg
	return "https://live.staticflickr.com/" + photo.Server + "/" + photo.ID + "_" + photo.Secret + "_w.jpg"
}

func flickrImageWebURL(photo FlickrPhoto) string {
	// https://www.flickr.com/photos/{owner-id}/{photo-id}
	return "https://www.flickr.com/photos/" + photo.Owner + "/" + photo.ID
}

func callFlickr(method string, resp any, params map[string]string) {
	params["method"] = method
	params["api_key"] = flickrAPIKey
	params["format"] = "json"
	params["nojsoncallback"] = "1"

	query := url.Values{}
	for k, v := range params {
		query.Set(k, v)
	}

	r := url.URL{
		Scheme:   "https",
		Host:     "www.flickr.com",
		Path:     "/services/rest",
		RawQuery: query.Encode(),
	}

	log.Printf("Calling Flickr API: %s", r.String())

	httpResp, err := http.Get(r.String())
	if err != nil {
		log.Fatal(err)
	}
	if httpResp.StatusCode != http.StatusOK {
		log.Fatalf("HTTP status %d", httpResp.StatusCode)
	}

	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		log.Fatal(err)
		return
	}

	err = json.Unmarshal([]byte(body), &resp)
	if err != nil {
		log.Fatal(err)
		return
	}
}

package flickr

import (
	"encoding/json"
	"github.com/joho/godotenv"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

var flickrAPIKey string

func init() {
	err := godotenv.Load(".env", ".local.env")
	if err != nil {
		log.Println(err)
	}

	flickrAPIKey = os.Getenv("FLICKR_API_KEY")
	if flickrAPIKey == "" {
		log.Fatal("FLICKR_API_KEY not set")
	}
}

type SearchResponse struct {
	Photos struct {
		Page    int     `json:"page"`
		Pages   int     `json:"pages"`
		PerPage int     `json:"perpage"`
		Total   int     `json:"total"`
		Photo   []Photo `json:"photo"`
	} `json:"photos"`
}

type Photo struct {
	ID     string `json:"id"`
	Owner  string `json:"owner"`
	Secret string `json:"secret"`
	Server string `json:"server"`
	Title  string `json:"title"`
}

func Call(method string, resp any, params map[string]string) {
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

	time.Sleep(time.Second)

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

	err = json.Unmarshal(body, &resp)
	if err != nil {
		log.Fatal(err)
		return
	}
}

/*
SourceURL returns the URL of the photo with the specified size.

Available sizes:

	s	thumbnail	75	cropped square
	q	thumbnail	150	cropped square
	t	thumbnail	100
	m	small	240
	n	small	320
	w	small	400
	(none)	medium	500
	z	medium	640
	c	medium	800
	b	large	1024
*/
func SourceURL(photo Photo, size string) string {
	// https://live.staticflickr.com/{server-id}/{id}_{secret}_{size-suffix}.jpg
	return "https://live.staticflickr.com/" + photo.Server + "/" + photo.ID + "_" + photo.Secret + "_" + size + ".jpg"
}

func WebURL(photo Photo) string {
	// https://www.flickr.com/photos/{owner-id}/{photo-id}
	return "https://www.flickr.com/photos/" + photo.Owner + "/" + photo.ID
}

package flickr

import (
	"encoding/json"
	"fmt"
	"github.com/joho/godotenv"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

var cacheDir = "/tmp/cg-flickr-cache"
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
	Page    int `json:"page"`
	Pages   int `json:"pages"`
	Perpage int `json:"perpage"`
	Total   int `json:"total"`
	Photos  struct {
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

	err := os.MkdirAll(cacheDir, 0755)
	if err != nil {
		log.Fatal(err)
	}

	cacheQuery, err := url.ParseQuery(query.Encode())
	if err != nil {
		log.Fatal(err)
	}
	cacheQuery.Del("api_key")

	cacheKey := hash(cacheQuery.Encode())
	cacheFile := fmt.Sprintf("%s/%d.json", cacheDir, cacheKey)

	if _, err := os.Stat(cacheFile); err == nil {
		file, err := os.Open(cacheFile)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		err = json.NewDecoder(file).Decode(resp)
		if err != nil {
			log.Fatal(err)
		}
		return
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

	file, err := os.Create(cacheFile)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	_, err = file.Write(body)
	if err != nil {
		log.Fatal(err)
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

func hash(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

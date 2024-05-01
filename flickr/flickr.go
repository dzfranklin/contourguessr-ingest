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
	"sync"
	"time"
)

var cacheDir = "/tmp/cg-flickr-cache"
var flickrApiKey string
var flickrEndpoint *url.URL

func init() {
	err := godotenv.Load(".env", ".env.local")
	if err != nil {
		log.Println(err)
	}

	flickrApiKey = os.Getenv("FLICKR_API_KEY")
	if flickrApiKey == "" {
		log.Fatal("FLICKR_API_KEY not set")
	}

	flickrEndpointS := os.Getenv("FLICKR_ENDPOINT")
	if flickrEndpointS == "" {
		log.Fatal("FLICKR_ENDPOINT not set")
	}
	flickrEndpoint, err = url.Parse(flickrEndpointS)
	if err != nil {
		log.Fatal(err)
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

	// only if you requested the extras
	DateUpload string `json:"dateupload"`
	Latitude   string `json:"latitude"`
	Longitude  string `json:"longitude"`
	Accuracy   string `json:"accuracy"`
}

var mu sync.Mutex
var lastCall time.Time

func Call(method string, resp any, params map[string]string) error {
	params["method"] = method
	params["format"] = "json"
	params["nojsoncallback"] = "1"

	query := url.Values{}
	for k, v := range params {
		query.Set(k, v)
	}

	r := *flickrEndpoint
	r.Path = "/services/rest"
	r.RawQuery = query.Encode()
	log.Println("flickr: ", r.String())

	mu.Lock()
	defer mu.Unlock()
	wait := time.Until(lastCall.Add(time.Second))
	if wait > 0 {
		time.Sleep(wait)
	}
	lastCall = time.Now()

	req, err := http.NewRequest(http.MethodGet, r.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", flickrApiKey)

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP status %d", httpResp.StatusCode)
	}

	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, &resp)
	if err != nil {
		return err
	}

	return nil
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

func SourceURLFromID(id string, size string) string {
	var details struct {
		Photo struct {
			ID     string `json:"id"`
			Server string `json:"server"`
			Secret string `json:"secret"`
		} `json:"photo"`
	}
	err := Call("flickr.photos.getInfo", &details, map[string]string{
		"photo_id": id,
	})
	if err != nil {
		log.Fatal(err)
	}

	photo := Photo{
		ID:     id,
		Server: details.Photo.Server,
		Secret: details.Photo.Secret,
	}

	return SourceURL(photo, size)
}

func hash(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

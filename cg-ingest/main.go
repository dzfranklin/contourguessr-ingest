package main

import (
	"bytes"
	"contourguessr-ingest/flickr"
	"encoding/json"
	"fmt"
	"github.com/jaytaylor/html2text"
	"github.com/joho/godotenv"
	flag "github.com/spf13/pflag"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

var visionKey string
var modelEndpoint string

var regions map[string]string
var numberToIngest = flag.IntP("n", "n", 5000, "Number of images to ingest")

func init() {
	regionsFile, err := os.Open("regions.json")
	if err != nil {
		log.Fatal(err)
	}
	defer regionsFile.Close()
	dec := json.NewDecoder(regionsFile)
	if err := dec.Decode(&regions); err != nil {
		log.Fatal(err)
	}

	// Environment variables

	err = godotenv.Load(".env", ".local.env")
	if err != nil {
		log.Println(err)
	}

	visionKey = os.Getenv("VISION_PREDICTION_KEY")
	if visionKey == "" {
		log.Fatal("VISION_PREDICTION_KEY not set")
	}
	modelEndpoint = os.Getenv("MODEL_PREDICTION_ENDPOINT")
	if modelEndpoint == "" {
		log.Fatal("MODEL_PREDICTION_ENDPOINT not set")
	}

	// Flags

	flag.Parse()
}

func main() {
	start := time.Date(2010, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	step := time.Hour * 24 * 30

	existing := make(map[string]struct{})
	existingCounts := make(map[string]int)
	picsFile, err := os.Open("pictures.ndjson")
	if err == nil {
		dec := json.NewDecoder(picsFile)
		for {
			var entry Entry
			if err := dec.Decode(&entry); err != nil {
				if err == io.EOF {
					break
				}
				log.Fatal(err)
			}
			existing[entry.Id] = struct{}{}
			existingCounts[entry.Region]++
		}
		picsFile.Close()
	} else if !os.IsNotExist(err) {
		log.Fatal(err)
	}

	picsFile, err = os.OpenFile("pictures.ndjson", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		log.Fatal(err)
	}
	defer picsFile.Close()
	picsEnc := json.NewEncoder(picsFile)

	for region, bbox := range regions {
		log.Printf("Processing region %s", region)

		var candidates []flickr.Photo
		for t := start; t.Before(end); t = t.Add(step) {
			log.Printf("Fetching images from %s to %s", t, t.Add(step))
			var search flickr.SearchResponse
			err := flickr.Call("flickr.photos.search", &search, map[string]string{
				"bbox":           bbox,
				"min_taken_date": fmt.Sprintf("%d", t.Unix()),
				"max_taken_date": fmt.Sprintf("%d", t.Add(step).Unix()),
				"sort":           "interestingness-desc",
				"safe_search":    "1",
				"per_page":       "500",
			})
			if err != nil {
				log.Fatal(err)
			}
			candidates = append(candidates, search.Photos.Photo...)
		}

		rand.Shuffle(len(candidates), func(i, j int) {
			candidates[i], candidates[j] = candidates[j], candidates[i]
		})

		classifier := NewClassifier()
		existingCount := existingCounts[region]
		pickCount := existingCount
		for n, candidate := range candidates {
			if _, ok := existing[candidate.ID]; ok {
				continue
			}

			if pickCount >= *numberToIngest {
				break
			}

			if !classifier.Classify(candidate) {
				continue
			}

			entry, err := createEntry(region, candidate.ID)
			if err != nil {
				log.Print(err)
				continue
			}
			pickCount++
			if err := picsEnc.Encode(entry); err != nil {
				log.Fatal(err)
			}

			log.Printf("picked %d of %d (%0.0f%%), scanned %d of %d (%0.0f%%), pick ratio %0.0f%%",
				pickCount, *numberToIngest, (float64(pickCount)/float64(*numberToIngest))*100.0,
				n+1, len(candidates), (float64(n+1)/float64(len(candidates)))*100.0,
				(float64(pickCount-existingCount)/float64(n+1))*100.0)
		}

		log.Printf("Picked %d images (target was %d)", pickCount, *numberToIngest)
	}

	log.Print("Done")
}

type Classifier struct {
	*http.Client
}

func NewClassifier() *Classifier {
	return &Classifier{
		Client: &http.Client{
			Timeout: time.Minute * 5,
		},
	}
}

func (c *Classifier) Classify(photo flickr.Photo) bool {
	reqPayload := struct {
		URL string `json:"url"`
	}{
		URL: flickr.SourceURL(photo, "m"),
	}
	reqPayloadJSON, err := json.Marshal(reqPayload)
	if err != nil {
		log.Fatal(err)
	}
	req, err := http.NewRequest("POST", modelEndpoint, bytes.NewBuffer(reqPayloadJSON))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prediction-Key", visionKey)
	resp, err := c.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	var prediction struct {
		Predictions []struct {
			Tag  string  `json:"tagName"`
			Prob float64 `json:"probability"`
		} `json:"predictions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prediction); err != nil {
		log.Fatal(err)
	}

	var negativeProb float64
	for _, p := range prediction.Predictions {
		if p.Tag == "Negative" {
			negativeProb = p.Prob
			break
		}
	}
	return negativeProb < 0.5
}

type PictureSize struct {
	Label  string `json:"label"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Source string `json:"source"`
}

type Entry struct {
	Id                  string        `json:"id"`
	Region              string        `json:"region"`
	R                   float64       `json:"r"`
	Sizes               []PictureSize `json:"sizes"`
	OwnerUsername       string        `json:"ownerUsername"`
	OwnerIcon           string        `json:"ownerIcon"`
	OwnerWebpage        string        `json:"ownerWebpage"`
	Title               string        `json:"title"`
	Description         string        `json:"description"`
	DateTaken           string        `json:"dateTaken"`
	Latitude            string        `json:"latitude"`
	Longitude           string        `json:"longitude"`
	LocationAccuracy    string        `json:"locationAccuracy"`
	LocationDescription string        `json:"locationDescription"`
	Webpage             string        `json:"url"`
}

func createEntry(region string, id string) (entry Entry, err error) {
	var info struct {
		Photo struct {
			Owner struct {
				NSID       string `json:"nsid"`
				Username   string `json:"username"`
				IconServer string `json:"iconserver"`
				IconFarm   int    `json:"iconfarm"`
			} `json:"owner"`
			Title struct {
				Content string `json:"_content"`
			} `json:"title"`
			Description struct {
				Content string `json:"_content"`
			} `json:"description"`
			Dates struct {
				Taken string `json:"taken"`
			} `json:"dates"`
			Location struct {
				Latitude     string `json:"latitude"`
				Longitude    string `json:"longitude"`
				Accuracy     string `json:"accuracy"`
				Neighborhood struct {
					Content string `json:"_content"`
				} `json:"neighborhood"`
				Locality struct {
					Content string `json:"_content"`
				} `json:"locality"`
				County struct {
					Content string `json:"_content"`
				} `json:"county"`
				Region struct {
					Content string `json:"_content"`
				} `json:"region"`
				Country struct {
					Content string `json:"_content"`
				} `json:"country"`
			} `json:"location"`
			URLs struct {
				URL []struct {
					Type    string `json:"type"`
					Content string `json:"_content"`
				} `json:"url"`
			} `json:"urls"`
		} `json:"photo"`
	}
	err = flickr.Call("flickr.photos.getInfo", &info, map[string]string{"photo_id": id})
	if err != nil {
		return
	}

	var sizes struct {
		Sizes struct {
			Size []PictureSize `json:"size"`
		}
	}
	err = flickr.Call("flickr.photos.getSizes", &sizes, map[string]string{"photo_id": id})
	if err != nil {
		return
	}

	ownerIcon := "https://www.flickr.com/images/buddyicon.gif"
	if info.Photo.Owner.IconServer != "0" {
		ownerIcon = "https://farm" + fmt.Sprintf("%d", info.Photo.Owner.IconFarm) + ".staticflickr.com/" + info.Photo.Owner.IconServer + "/buddyicons/" + info.Photo.Owner.NSID + ".jpg"
	}

	potentialLocationSegments := []string{
		info.Photo.Location.Neighborhood.Content, info.Photo.Location.Locality.Content,
		info.Photo.Location.County.Content, info.Photo.Location.Region.Content, info.Photo.Location.Country.Content}
	var locationSegments []string
	for _, segment := range potentialLocationSegments {
		if segment != "" {
			locationSegments = append(locationSegments, segment)
		}
	}
	locationDescription := strings.Join(locationSegments, ", ")

	ownerWebpage := "https://flickr.com/photos/" + info.Photo.Owner.NSID

	webpage := ownerWebpage
	if len(info.Photo.URLs.URL) > 0 {
		webpage = info.Photo.URLs.URL[0].Content
	}

	r := rand.Float64()

	var description string
	description, err = html2text.FromString(info.Photo.Description.Content, html2text.Options{})
	if err != nil {
		return
	}

	entry = Entry{
		Id:                  "flickr:" + id,
		Region:              region,
		R:                   r,
		Sizes:               sizes.Sizes.Size,
		OwnerUsername:       info.Photo.Owner.Username,
		OwnerIcon:           ownerIcon,
		OwnerWebpage:        ownerWebpage,
		Title:               info.Photo.Title.Content,
		Description:         description,
		DateTaken:           info.Photo.Dates.Taken,
		Latitude:            info.Photo.Location.Latitude,
		Longitude:           info.Photo.Location.Longitude,
		LocationAccuracy:    info.Photo.Location.Accuracy,
		LocationDescription: locationDescription,
		Webpage:             webpage,
	}
	return
}

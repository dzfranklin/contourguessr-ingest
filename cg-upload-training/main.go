package main

import (
	"bytes"
	"context"
	"contourguessr-ingest/flickr"
	"encoding/json"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/services/cognitiveservices/v3.0/customvision/training"
	"github.com/gofrs/uuid"
	"github.com/joho/godotenv"
	flag "github.com/spf13/pflag"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

var trainedFile = "./trained.ndjson"

var visionKey string
var visionEndpoint string
var visionProjectID string

var query = flag.StringP("query", "q", "", "Flickr search query")
var targetCount = flag.IntP("count", "c", 10, "Number of images to ingest per region")
var regionFilter = flag.StringP("region-filter", "r", "", "Prefix of region IDs to ingest")
var noBbox = flag.Bool("no-bbox", false, "Don't filter by bounding box")

var regionList []Region

type Region struct {
	Id   string `json:"id"`
	BBox string `json:"bbox"`
}

func init() {
	regionsFile, err := os.Open("regions.json")
	if err != nil {
		log.Fatal(err)
	}
	defer regionsFile.Close()
	dec := json.NewDecoder(regionsFile)
	var regions map[string]string
	if err := dec.Decode(&regions); err != nil {
		log.Fatal(err)
	}

	for id, bbox := range regions {
		regionList = append(regionList, Region{Id: id, BBox: bbox})
	}
	sort.Slice(regionList, func(i, j int) bool {
		return regionList[i].Id < regionList[j].Id
	})

	// Environment variables

	err = godotenv.Load(".env", ".env.local")
	if err != nil {
		log.Println(err)
	}

	visionKey = os.Getenv("VISION_TRAINING_KEY")
	if visionKey == "" {
		log.Fatal("VISION_TRAINING_KEY not set")
	}
	visionEndpoint = os.Getenv("VISION_TRAINING_ENDPOINT")
	if visionEndpoint == "" {
		log.Fatal("VISION_TRAINING_ENDPOINT not set")
	}
	visionProjectID = os.Getenv("VISION_PROJECT_ID")
	if visionProjectID == "" {
		log.Fatal("VISION_PROJECT_ID not set")
	}

	// Flags

	flag.Parse()
}

func main() {
	ctx := context.Background()

	trainer := training.New(visionKey, visionEndpoint)
	project, err := uuid.FromString(visionProjectID)
	if err != nil {
		log.Fatal(err)
	}

	alreadyTrained := readTrained()
	trainedAppend, err := os.OpenFile(trainedFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	trainedEnc := json.NewEncoder(trainedAppend)
	defer trainedAppend.Close()

	h := http.Client{
		Timeout: time.Second * 10,
	}

	for _, region := range regionList {
		if *regionFilter != "" && !strings.HasPrefix(region.Id, *regionFilter) {
			log.Printf("Skipping %s", region.Id)
			continue
		}
		log.Printf("Searching %s", region.Id)

		// Search for candidates
		var candidates []flickr.Photo
		for year := 2023; year >= 2019; year-- {
			stepSize := 3
			for month := 12; month >= 1; month = month - stepSize {
				start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
				end := start.AddDate(0, stepSize, 0)

				bbox := region.BBox
				if *noBbox {
					bbox = ""
				}

				var search flickr.SearchResponse
				err := flickr.Call("flickr.photos.search", &search, map[string]string{
					"bbox":           bbox,
					"min_taken_date": fmt.Sprintf("%d", start.Unix()),
					"max_taken_date": fmt.Sprintf("%d", end.Unix()),
					"sort":           "interestingness-desc",
					"per_page":       "500",
					"text":           *query,
				})
				if err != nil {
					log.Fatal(err)
				}

				for _, photo := range search.Photos.Photo {
					if !contains(alreadyTrained, photo.ID) {
						candidates = append(candidates, photo)
					}
				}
			}

			log.Printf("Searched %d, %0.0f%% done", year, min((float64(len(candidates))/float64(*targetCount))*100, 100))

			if len(candidates) >= *targetCount {
				break
			}
		}

		// Pick candidates at random
		rand.Shuffle(len(candidates), func(i, j int) {
			candidates[i], candidates[j] = candidates[j], candidates[i]
		})
		picks := candidates[:min(*targetCount, len(candidates))]

		// Upload

		for _, pick := range picks {
			src := flickr.SourceURL(pick, "m")

			// Download
			req, err := http.NewRequest(http.MethodGet, src, nil)
			if err != nil {
				log.Fatal(err)
			}
			req.Header.Set("User-Agent", "contourguessr.org (contact daniel@danielzfranklin.org)")
			resp, err := h.Do(req)
			if err != nil {
				log.Fatal(err)
			}
			imgBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Fatal(err)
			}

			// Upload
			_, err = trainer.CreateImagesFromData(ctx, project, io.NopCloser(bytes.NewReader(imgBytes)), []uuid.UUID{})
			if err != nil {
				log.Fatal(err)
			}

			// Mark trained
			pushTrained(trainedEnc, pick.ID)
		}

		if *noBbox {
			break
		}

		log.Printf("Uploaded %d images from %s (%d candidates, target was %d)", len(picks), region.Id, len(candidates), *targetCount)
	}

	log.Println("Done")
}

type trainedFileEntry struct {
	FlickrID string `json:"flickrID"`
}

func contains(set map[string]struct{}, id string) bool {
	_, ok := set[id]
	return ok
}

func readTrained() map[string]struct{} {
	f, err := os.Open(trainedFile)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]struct{})
		}
		log.Fatal(err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	entries := make(map[string]struct{})
	for {
		var entry trainedFileEntry
		if err := dec.Decode(&entry); err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}
		entries[entry.FlickrID] = struct{}{}
	}
	return entries
}

func pushTrained(enc *json.Encoder, entry string) {
	if err := enc.Encode(trainedFileEntry{FlickrID: entry}); err != nil {
		log.Fatal(err)
	}
}

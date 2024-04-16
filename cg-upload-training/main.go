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
	"time"
)

var trainedFile = "./trained.ndjson"

var visionKey string
var visionEndpoint string
var visionProjectID string

var region = flag.String("region", "", "Region to search in")
var bbox string

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

	// Environment variables

	err = godotenv.Load(".env", ".local.env")
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

	if *region == "" {
		log.Fatal("region missing")
	}
	var ok bool
	bbox, ok = regions[*region]
	if !ok {
		log.Fatalf("unknown region %q", *region)
	}
}

func main() {
	start := time.Date(2019, time.January, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(4, 0, 0)
	step := time.Hour * 24 * 7
	//step := time.Hour * 24 * 7 * 30 * 3
	pickPerStep := 5

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

	for t := start; t.Before(end); t = t.Add(step) {
		log.Println("Searching for photos from", t, "to", t.Add(step))

		// Search for photos within the step

		var search flickr.SearchResponse
		flickr.Call("flickr.photos.search", &search, map[string]string{
			"bbox":           bbox,
			"min_taken_date": fmt.Sprintf("%d", t.Unix()),
			"max_taken_date": fmt.Sprintf("%d", t.Add(step).Unix()),
			"sort":           "date-uploaded-asc",
			"per_page":       "500",
			//"text":           "bw",
			//"text": "cloud",
			//"text": "mist",
			//"text": "fog",
			//"text": "road",
		})

		// Find candidates we haven't already used

		var candidates []flickr.Photo
		for _, photo := range search.Photos.Photo {
			if !contains(alreadyTrained, photo.ID) {
				candidates = append(candidates, photo)
			}
		}

		// Pick candidates at random

		rand.Shuffle(len(candidates), func(i, j int) {
			candidates[i], candidates[j] = candidates[j], candidates[i]
		})
		picks := candidates[:min(pickPerStep, len(candidates))]
		log.Printf("Randomly picked %d out of %d candidates", len(picks), len(candidates))

		// Download and upload each

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

			_, err = trainer.CreateImagesFromData(ctx, project, io.NopCloser(bytes.NewReader(imgBytes)), []uuid.UUID{})
			if err != nil {
				log.Fatal(err)
			}

			// Mark as trained

			pushTrained(trainedEnc, pick.ID)
		}
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

package main

import (
	"bytes"
	"context"
	"contourguessr-ingest/flickr"
	"encoding/json"
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
	"sync"
)

var trainedFile = "./trained.ndjson"

var visionKey string
var visionEndpoint string
var visionProjectID string

var regionList []Region

type Region struct {
	Id   string `json:"id"`
	BBox string `json:"bbox"`
}

var numToTrain = flag.IntP("number", "n", 100, "Number to train")

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
}

func main() {
	ctx := context.Background()

	trainer := training.New(visionKey, visionEndpoint)
	project, err := uuid.FromString(visionProjectID)
	if err != nil {
		log.Fatal(err)
	}

	trainedAppend, err := os.OpenFile(trainedFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	trainedEnc := json.NewEncoder(trainedAppend)
	defer trainedAppend.Close()

	tags, err := trainer.GetTags(ctx, project, nil)
	if err != nil {
		log.Fatal(err)
	}
	var yTag *uuid.UUID
	var nTag *uuid.UUID
	for _, tag := range *tags.Value {
		if *tag.Name == "Positive" {
			yTag = tag.ID
		} else if *tag.Name == "Negative" {
			nTag = tag.ID
		}
	}
	if yTag == nil || nTag == nil {
		log.Fatal("Failed to load tags")
	}

	var mu sync.Mutex

	totalLabelled := 0

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.Method {
		case http.MethodGet:
			htmlFile, err := os.Open("cg-label/index.html")
			if err != nil {
				log.Fatal(err)
			}
			defer htmlFile.Close()
			html, err := io.ReadAll(htmlFile)
			if err != nil {
				log.Fatal(err)
			}

			batchPayloadJSON := selectBatchPayload()

			html = bytes.ReplaceAll(html, []byte("{{ batchPayload }}"), []byte(batchPayloadJSON))

			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write(html)
		case http.MethodPost:
			var payload []struct {
				Photo flickr.Photo `json:"photo"`
				Tag   string       `json:"tag"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			var images []training.ImageURLCreateEntry
			for _, entry := range payload {
				var tag uuid.UUID
				switch entry.Tag {
				case "y":
					tag = *yTag
				case "n":
					tag = *nTag
				}

				url := flickr.SourceURL(entry.Photo, "m")
				images = append(images, training.ImageURLCreateEntry{
					URL:    &url,
					TagIds: &[]uuid.UUID{tag},
				})
			}

			uploadBatchSize := 64
			for i := 0; i < len(images); i += uploadBatchSize {
				batch := images[i:min(i+uploadBatchSize, len(images))]
				_, err := trainer.CreateImagesFromUrls(ctx, project, training.ImageURLCreateBatch{
					Images: &batch,
				})
				if err != nil {
					log.Fatal(err)
				}
			}

			for _, entry := range payload {
				pushTrained(trainedEnc, entry.Photo.ID)
			}

			totalLabelled += len(payload)
			log.Printf("%d images tagged", totalLabelled)
		}
	})

	addr := "localhost:4000"
	log.Println("Visit", "http://"+addr, "to label images.")

	err = http.ListenAndServe(addr, mux)
	if err != nil {
		panic(err)
	}
}

func selectBatchPayload() string {
	alreadyTrained := readTrained()

	candidatesFile, err := os.Open("candidates.ndjson")
	if err != nil {
		log.Fatal(err)
	}
	candidatesDec := json.NewDecoder(candidatesFile)
	var candidates []flickr.Photo
	for {
		var entry flickr.Photo
		if err := candidatesDec.Decode(&entry); err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}
		if !contains(alreadyTrained, entry.ID) {
			candidates = append(candidates, entry)
		}
	}
	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	batch := candidates[:min(len(candidates), *numToTrain)]

	var batchPayload []batchPayloadEntry
	for _, entry := range batch {
		entry.Title = "" // hack to avoid properly escaping as we don't need
		batchPayload = append(batchPayload, batchPayloadEntry{
			Source:  flickr.SourceURL(entry, "c"),
			Webpage: "https://www.flickr.com/photos/" + entry.Owner + "/" + entry.ID,
			Photo:   entry,
		})
	}
	batchPayloadJSON, err := json.Marshal(batchPayload)
	if err != nil {
		log.Fatal(err)
	}

	return string(batchPayloadJSON)
}

type batchPayloadEntry struct {
	Source  string `json:"source"`
	Webpage string `json:"webpage"`
	flickr.Photo
}

type trainedFileEntry struct {
	FlickrID string `json:"flickrID"`
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

func contains(set map[string]struct{}, id string) bool {
	_, ok := set[id]
	return ok
}

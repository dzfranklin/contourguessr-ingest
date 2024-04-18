package main

import (
	"contourguessr-ingest/flickr"
	"encoding/json"
	"fmt"
	"github.com/joho/godotenv"
	flag "github.com/spf13/pflag"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"time"
)

var trainedFile = "./trained.ndjson"

var regionList []Region

type Region struct {
	Id   string `json:"id"`
	BBox string `json:"bbox"`
}

var regionFilter = flag.StringP("region-filter", "r", "", "Prefix of region IDs to ingest")

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
}

func main() {
	alreadyTrained := readTrained()

	var candidates []flickr.Photo
	for _, region := range regionList {
		if *regionFilter != "" && !strings.HasPrefix(region.Id, *regionFilter) {
			log.Printf("Skipping %s", region.Id)
			continue
		}

		// Search for candidates
		for year := 2023; year >= 2013; year-- {
			log.Printf("Searching %s: %d", region.Id, year)
			stepSize := 3
			for month := 12; month >= 1; month = month - stepSize {
				start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
				end := start.AddDate(0, stepSize, 0)
				bbox := region.BBox

				var search flickr.SearchResponse
				err := flickr.Call("flickr.photos.search", &search, map[string]string{
					"bbox":           bbox,
					"min_taken_date": fmt.Sprintf("%d", start.Unix()),
					"max_taken_date": fmt.Sprintf("%d", end.Unix()),
					"sort":           "interestingness-desc",
					"per_page":       "500",
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
		}
	}

	log.Printf("Found %d candidates", len(candidates))

	outFile, err := os.Create("candidates.ndjson")
	if err != nil {
		log.Fatal(err)
	}
	defer outFile.Close()
	enc := json.NewEncoder(outFile)
	for _, photo := range candidates {
		err := enc.Encode(photo)
		if err != nil {
			log.Fatal(err)
		}
	}

	log.Println("Wrote to candidates.ndjson")
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

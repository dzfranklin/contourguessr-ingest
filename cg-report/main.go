package main

import (
	"context"
	"contourguessr-ingest/flickr"
	"github.com/Azure/azure-sdk-for-go/services/cognitiveservices/v3.0/customvision/training"
	"github.com/gofrs/uuid"
	"github.com/joho/godotenv"
	flag "github.com/spf13/pflag"
	"log"
	"net/url"
	"os"
	"strings"
)

var visionKey string
var visionEndpoint string
var visionProjectID string

var urlToReport = flag.StringP("url", "u", "", "Share link to specific challenge to report")

func init() {
	// Environment variables

	err := godotenv.Load(".env", ".env.local")
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
		log.Fatal("parse vision project id: ", err)
	}

	parsedURL, err := url.Parse(*urlToReport)
	if err != nil {
		log.Fatal(err)
	}

	serializedId := parsedURL.Query().Get("p")
	if serializedId == "" {
		log.Fatal("No picture ID found in URL")
	}

	id, err := url.QueryUnescape(serializedId)
	if err != nil {
		log.Fatal(err)
	}

	if !strings.HasPrefix(id, "flickr:") {
		log.Fatal("Unsupported picture ID format")
	}
	id = strings.TrimPrefix(id, "flickr:")

	flickrURL := flickr.SourceURLFromID(id, "w")

	var negativeTag uuid.UUID
	tags, err := trainer.GetTags(ctx, project, nil)
	if err != nil {
		log.Fatal(err)
	}
	for _, tag := range *tags.Value {
		if *tag.Name == "Negative" {
			negativeTag = *tag.ID
			break
		}
	}
	if negativeTag == (uuid.UUID{}) {
		log.Fatal("Negative tag not found")
	}

	_, err = trainer.CreateImagesFromUrls(ctx, project, training.ImageURLCreateBatch{
		Images: &[]training.ImageURLCreateEntry{
			{
				URL:    &flickrURL,
				TagIds: &[]uuid.UUID{negativeTag},
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Reported %s as negative", flickrURL)
}

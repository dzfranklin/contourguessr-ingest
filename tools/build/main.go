package main

import (
	"context"
	"contourguessr-ingest/repos"
	"encoding/json"
	"fmt"
	"github.com/joho/godotenv"
	"log"
	"log/slog"
	"os"
	"strconv"
	"time"
)

var repo *repos.Repo

func main() {
	err := godotenv.Load(".env", ".env.local")
	if err != nil {
		slog.Info("no dotenv", "err", err)
	}

	databaseURL := mustGetEnv("DATABASE_URL")
	repo, err = repos.Connect(databaseURL)
	if err != nil {
		log.Fatal(err)
	}

	err = doMain(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	slog.Info("completed")
}

func doMain(ctx context.Context) error {
	var batch []repos.Photo
	var err error
	cursor := ""
	for {
		cursor, batch, err = repo.ListOKPhotos(ctx, cursor)
		if err != nil {
			return err
		}

		if len(batch) == 0 {
			break
		}

		for _, photo := range batch {
			err = processPhoto(ctx, photo)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type infoData struct {
	Owner struct {
		Iconfarm   int
		Iconserver string
		NSID       string
		Username   string
		PathAlias  string `json:"path_alias"`
	}
	Title       stringContent
	Description stringContent
	Dates       struct {
		Taken string
	}
}

type stringContent struct {
	Content string `json:"_content"`
}

func processPhoto(ctx context.Context, photo repos.Photo) error {
	var info infoData
	err := json.Unmarshal(photo.Info, &info)
	if err != nil {
		return fmt.Errorf("unmarshal info: %w", err)
	}

	lng, lat, err := photo.ParseLngLat()
	if err != nil {
		return fmt.Errorf("parse lng lat: %w", err)
	}

	owner := info.Owner

	var photographerIcon string
	if owner.Iconfarm > 0 {
		photographerIcon = "https://farm" + strconv.Itoa(owner.Iconfarm) + ".staticflickr.com/" + owner.Iconserver + "/buddyicons/" + owner.NSID + ".jpg"
	} else {
		photographerIcon = "https://combo.staticflickr.com/pw/images/buddyicon03.png"
	}

	photographerText := owner.Username

	var photographerLink, link string
	if owner.PathAlias != "" {
		photographerLink = "https://www.flickr.com/people/" + owner.PathAlias
		link = "https://www.flickr.com/photos/" + owner.PathAlias + "/" + photo.Id
	} else {
		photographerLink = "https://www.flickr.com/people/" + owner.NSID
		link = "https://www.flickr.com/photos/" + owner.NSID + "/" + photo.Id
	}

	title := info.Title.Content
	descriptionHtml := info.Description.Content

	var dateTaken *time.Time
	if info.Dates.Taken == "" {
		dateTaken = nil
	} else if value, err := time.Parse("2006-01-02 15:04:05", info.Dates.Taken); err == nil {
		dateTaken = &value
	} else {
		return fmt.Errorf("failed to parse date: %v", err)
	}

	err = repo.InsertChallenge(ctx, repos.InsertChallengeData{
		FlickrId:         photo.Id,
		RegionId:         photo.RegionId,
		Lng:              lng,
		Lat:              lat,
		RegularSrc:       photo.Medium.Source,
		RegularWidth:     photo.Medium.Width,
		RegularHeight:    photo.Medium.Height,
		LargeSrc:         photo.Large.Source,
		LargeWidth:       photo.Large.Width,
		LargeHeight:      photo.Large.Height,
		PhotographerIcon: photographerIcon,
		PhotographerText: photographerText,
		PhotographerLink: photographerLink,
		Title:            title,
		DescriptionHTML:  descriptionHtml,
		DateTaken:        dateTaken,
		Link:             link,
	})
	if err != nil {
		return fmt.Errorf("insert challenge: %w", err)
	}

	return nil
}

func mustGetEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("%s not set", key)
	}
	return value
}

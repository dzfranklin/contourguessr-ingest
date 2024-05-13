package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

var fetchFlickrPhotoMu sync.Mutex

func fetchFlickrPhoto(db *pgx.Conn, flickrId string, photoURL string) ([]byte, error) {
	startTime := time.Now()

	fetchFlickrPhotoMu.Lock()
	defer fetchFlickrPhotoMu.Unlock()

	randSleep(minFlickrImgRequestDelay, maxFlickrImgRequestDelay)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	imgReq, err := http.NewRequestWithContext(ctx, "GET", photoURL, nil)
	if err != nil {
		return nil, err
	}
	imgReq.Header.Set("User-Agent", "contourguessr.org (contact github.com/dzfranklin/contourguessr-ingest or daniel@danielzfranklin.org)")

	reqTime := time.Now()
	imgResp, err := http.DefaultClient.Do(imgReq)
	if err != nil {
		saveFlickrPhotoFetchFailure(db, flickrId, fmt.Errorf("request: %w", err))
		return nil, err
	}
	defer imgResp.Body.Close()

	if imgResp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(imgResp.Body)
		if err != nil {
			body = []byte(fmt.Sprintf("<error reading body: %s>", err))
		}
		err = fmt.Errorf("HTTP status %d: %s", imgResp.StatusCode, body)
		saveFlickrPhotoFetchFailure(db, flickrId, err)
		return nil, err
	}

	body, err := io.ReadAll(imgResp.Body)
	if err != nil {
		saveFlickrPhotoFetchFailure(db, flickrId, fmt.Errorf("read body: %w", err))
		return nil, err
	}

	log.Printf("Fetched flickr photo %s in %s (%s requesting)",
		photoURL, time.Since(startTime), time.Since(reqTime))

	return body, nil
}

func randSleep(min time.Duration, max time.Duration) {
	dur := time.Duration(rand.Int63n(int64(max-min))) + min
	if dur > 5*time.Minute {
		log.Printf("Sleeping for %s", dur)
	}
	time.Sleep(dur)
}

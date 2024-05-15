package compute_features

import (
	"context"
	"contourguessr-ingest/compute_features/elevation"
	"contourguessr-ingest/compute_features/ml"
	"contourguessr-ingest/compute_features/roads"
	"contourguessr-ingest/overpass"
	"contourguessr-ingest/repos"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"time"
)

const maxConcurrency = 5

type Feature = repos.Features

type result struct {
	photoID string
	feature Feature
	err     error
}

func Compute(
	ctx context.Context,
	bing *elevation.BingMaps,
	ml *ml.Client,
	ovp *overpass.Client,
	photos []repos.Photo,
) ([]Feature, error) {
	httpC := &http.Client{}

	results := make(chan result, len(photos))
	sem := make(chan struct{}, maxConcurrency)
	for _, photo := range photos {
		select {
		case sem <- struct{}{}:
			go func() {
				defer func() { <-sem }()
				f, err := computeOne(ctx, httpC, bing, ml, ovp, photo)
				results <- result{photo.Id, f, err}
			}()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	var features []Feature
	var errs []error
	for range photos {
		r := <-results
		if r.err != nil {
			slog.Warn("compute failed", "photo_id", r.photoID, "err", r.err)
			errs = append(errs, r.err)
		} else {
			features = append(features, r.feature)
		}
	}

	if len(errs) > min(10, len(photos)/10) {
		return nil, fmt.Errorf("too many errors: %d/%d failed: %v", len(errs), len(photos), errs)
	}
	return features, nil
}

func computeOne(
	ctx context.Context,
	httpC *http.Client,
	bing *elevation.BingMaps,
	ml *ml.Client,
	ovp *overpass.Client,
	photo repos.Photo,
) (Feature, error) {
	out := Feature{PhotoId: photo.Id}

	lng, lat, err := photo.ParseLngLat()
	if err != nil {
		return Feature{}, fmt.Errorf("parse lnglat: %w", err)
	}

	exif, err := photo.ParseExif()
	if err != nil {
		return Feature{}, fmt.Errorf("parse exif: %w", err)
	}

	bingStart := time.Now()
	terrainElevation, err := bing.LookupElevation(ctx, lng, lat)
	if err != nil {
		return Feature{}, fmt.Errorf("lookup elevation: %w", err)
	}
	out.TerrainElevationMeters = int(math.Round(terrainElevation))
	slog.Debug("bing lookup", "duration_secs", time.Since(bingStart).Seconds())

	gpsAltitude, err := elevation.ParseGPSAltitude(exif)
	if err != nil {
		slog.Info("failed to parse GPS altitude", "photo_id", photo.Id, "err", err)
		out.NoGPSAltitude = true
	} else {
		out.GPSAltitudeMeters = int(math.Round(gpsAltitude))
	}

	fetchPhotoStart := time.Now()
	photoData, err := fetchPhotoData(ctx, httpC, photo)
	if err != nil {
		return Feature{}, fmt.Errorf("fetch photo data: %w", err)
	}
	slog.Debug("fetch photo", "duration_secs", time.Since(fetchPhotoStart).Seconds())

	validityStart := time.Now()
	validity, err := ml.PredictValidity(ctx, photoData)
	if err != nil {
		return Feature{}, fmt.Errorf("predict validity: %w", err)
	}
	out.ValidityScore = validity.Score
	out.ValidityModel = validity.Model
	slog.Debug("validity predict", "duration_secs", time.Since(validityStart).Seconds())

	distanceStart := time.Now()
	out.NearestRoadMeters, err = roads.DistanceToRoad(ctx, ovp, lng, lat)
	if err != nil {
		return Feature{}, fmt.Errorf("nearest road: %w", err)
	}
	slog.Debug("nearest road", "duration_secs", time.Since(distanceStart).Seconds())

	return out, nil
}

func fetchPhotoData(ctx context.Context, httpC *http.Client, photo repos.Photo) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", photo.Medium.Source, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := httpC.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return data, nil
}

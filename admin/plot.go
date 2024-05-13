package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type plotPoint struct {
	FlickrID   string    `json:"flickr_id"`
	PreviewURL string    `json:"preview_url"`
	WebURL     string    `json:"web_url"`
	Geo        []float64 `json:"geo"`
}

func plotHandler(w http.ResponseWriter, r *http.Request) {
	// Parse params
	q := r.URL.Query()
	selectedRegionS := q.Get("region")

	regions, err := listRegions(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var regionGeoJSON string
	var regionBBoxJSON string
	pointsJSON := []byte("null")
	var totalCount int
	if selectedRegionS != "" {
		selectedRegion, err := strconv.Atoi(selectedRegionS)
		if err != nil {
			http.Error(w, "invalid region_id", http.StatusBadRequest)
			return
		}

		regionGeoJSON, regionBBoxJSON, err = loadRegionGeo(r.Context(), selectedRegion)
		if err != nil {
			http.Error(w, fmt.Sprintf("error loading region geo: %v", err), http.StatusInternalServerError)
			return
		}

		points, err := loadPoints(r.Context(), selectedRegion)
		if err != nil {
			http.Error(w, fmt.Sprintf("error loading points: %v", err), http.StatusInternalServerError)
			return
		}

		pointsJSON, err = json.Marshal(points)
		if err != nil {
			http.Error(w, fmt.Sprintf("error marshalling points: %v", err), http.StatusInternalServerError)
			return
		}

		totalCount = len(points)
	}

	templateResponse(w, r, "plot.tmpl.html", M{
		"MaptilerAPIKey": mtApiKey,
		"Regions":        regions,
		"SelectedRegion": selectedRegionS,
		"RegionBBoxJSON": regionBBoxJSON,
		"RegionGeoJSON":  regionGeoJSON,
		"PointsJSON":     string(pointsJSON),
		"TotalCount":     totalCount,
	})
}

func loadRegionGeo(ctx context.Context, region int) (string, string, error) {
	var geom string
	var bbox string
	err := db.QueryRow(ctx, `
		SELECT ST_AsGeoJSON(geo::geometry), ST_AsGeoJSON(ST_Envelope(geo::geometry))
		FROM regions
		WHERE id = $1
	`, region).Scan(&geom, &bbox)
	if err != nil {
		return "", "", err
	}
	return geom, bbox, nil
}

func loadPoints(ctx context.Context, region int) ([]plotPoint, error) {
	points := make([]plotPoint, 0)
	rows, err := db.Query(ctx, `
		SELECT f.id, f.info->'owner'->>'nsid', f.medium->>'source',
		       (f.info->'location'->>'longitude')::float, (f.info->'location'->>'latitude')::float
		FROM flickr f
		WHERE f.region_id = $1
	`, region)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p plotPoint
		var owner string
		var lng, lat float64
		err = rows.Scan(&p.FlickrID, &owner, &p.PreviewURL,
			&lng, &lat)
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		p.WebURL = "https://www.flickr.com/photos/" + owner + "/" + p.FlickrID
		p.Geo = []float64{lng, lat}
		points = append(points, p)
	}
	return points, nil
}

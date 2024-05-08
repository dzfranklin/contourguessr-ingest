package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type plotPoint struct {
	FlickrID       string    `json:"flickr_id"`
	PreviewURL     string    `json:"preview_url"`
	WebURL         string    `json:"web_url"`
	Geo            []float64 `json:"geo"`
	IsAccepted     bool      `json:"is_accepted"`
	ScoreUpdatedAt time.Time `json:"score_updated_at"`
	ValidityScore  float64   `json:"validity_score"`
	ValidityModel  string    `json:"validity_model"`
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
	var validCount int
	var totalCount int
	if selectedRegionS != "" {
		selectedRegion, err := strconv.Atoi(selectedRegionS)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		regionGeoJSON, regionBBoxJSON, err = loadRegionGeo(r.Context(), selectedRegion)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		points, err := loadPoints(r.Context(), selectedRegion)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		pointsJSON, err = json.Marshal(points)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for _, point := range points {
			if point.ValidityScore >= 0.5 {
				validCount++
			}
			totalCount++
		}
	}

	templateResponse(w, r, "plot.tmpl.html", M{
		"MaptilerAPIKey": MaptilerAPIKey,
		"Regions":        regions,
		"SelectedRegion": selectedRegionS,
		"RegionBBoxJSON": regionBBoxJSON,
		"RegionGeoJSON":  regionGeoJSON,
		"PointsJSON":     string(pointsJSON),
		"ValidCount":     validCount,
		"TotalCount":     totalCount,
		"ValidPercent":   fmt.Sprintf("%.2f%%", float64(validCount)/float64(totalCount)*100),
	})
}

func loadRegionGeo(ctx context.Context, region int) (string, string, error) {
	var geom string
	var bbox string
	err := Db.QueryRow(ctx, `
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
	rows, err := Db.Query(ctx, `
		SELECT p.flickr_id, p.summary->>'owner', p.summary->>'server', p.summary->>'secret',
		       ST_X(p.geo::geometry), ST_Y(p.geo::geometry),
		       s.is_accepted, s.updated_at, s.validity_score, s.validity_model
		FROM flickr_photos as p
				 JOIN (SELECT DISTINCT ON (flickr_photo_id) *
					   FROM photo_scores
					   ORDER BY flickr_photo_id, updated_at DESC) AS s ON p.flickr_id = s.flickr_photo_id
		WHERE not road_within_1000m and region_id = $1
	`, region)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p plotPoint
		var owner string
		var server string
		var secret string
		var lng, lat float64
		err = rows.Scan(&p.FlickrID, &owner, &server, &secret,
			&lng, &lat,
			&p.IsAccepted, &p.ScoreUpdatedAt, &p.ValidityScore, &p.ValidityModel)
		if err != nil {
			return nil, err
		}
		p.PreviewURL = "https://live.staticflickr.com/" + server + "/" + p.FlickrID + "_" + secret + "_n.jpg"
		p.WebURL = "https://www.flickr.com/photos/" + owner + "/" + p.FlickrID
		p.Geo = []float64{lng, lat}
		points = append(points, p)
	}
	return points, nil
}

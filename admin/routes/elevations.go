package routes

import (
	"context"
	"net/http"
)

func elevationsHandler(w http.ResponseWriter, r *http.Request) {
	total, err := countTotalWithAlti(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	histogram := []histogramEntry{
		{Min: -100000, Max: -500},
		{Min: -500, Max: -400},
		{Min: -400, Max: -300},
		{Min: -300, Max: -200},
		{Min: -200, Max: -100},
		{Min: -100, Max: 0},
		{Min: 0, Max: 100},
		{Min: 100, Max: 200},
		{Min: 200, Max: 300},
		{Min: 300, Max: 400},
		{Min: 400, Max: 500},
		{Min: 500, Max: 100000},
	}
	for i := range histogram {
		entry, err := loadHistogramEntry(r.Context(), histogram[i].Min, histogram[i].Max)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		histogram[i] = entry
	}

	templateResponse(w, r, "elevations.tmpl.html", M{
		"Total":     total,
		"Histogram": histogram,
	})
}

func countTotalWithAlti(ctx context.Context) (int, error) {
	var count int
	err := Db.QueryRow(ctx, `
		SELECT count(*)
		FROM photo_scores
		WHERE gps_altitude_available
	`).Scan(&count)
	return count, err
}

type histogramEntry struct {
	Min      int
	Max      int
	Count    int
	Examples []histogramPoint
}

type histogramPoint struct {
	FlickrID        string
	PreviewURL      string
	WebURL          string
	GPSAltitude     float64
	TerrainAltitude float64
}

func loadHistogramEntry(ctx context.Context, minAlt int, maxAlt int) (histogramEntry, error) {
	rows, err := Db.Query(ctx, `
		SELECT p.flickr_id, p.summary->>'owner', p.summary->>'server', p.summary->>'secret',
			   s.gps_altitude, s.terrain_altitude
		FROM flickr_photos as p
		JOIN photo_scores as s ON s.flickr_photo_id = p.flickr_id
		WHERE s.gps_altitude_available AND
		    (s.gps_altitude - s.terrain_altitude) >= $1 AND
			(s.gps_altitude - s.terrain_altitude) < $2
		ORDER BY random()
	`, minAlt, maxAlt)
	if err != nil {
		return histogramEntry{}, err
	}
	defer rows.Close()

	var points []histogramPoint
	for rows.Next() {
		var flickrID string
		var owner string
		var server string
		var secret string
		var gpsAltitude float64
		var terrainAltitude float64
		err = rows.Scan(&flickrID, &owner, &server, &secret, &gpsAltitude, &terrainAltitude)
		if err != nil {
			return histogramEntry{}, err
		}

		points = append(points, histogramPoint{
			FlickrID:        flickrID,
			PreviewURL:      "https://live.staticflickr.com/" + server + "/" + flickrID + "_" + secret + "_n.jpg",
			WebURL:          "https://www.flickr.com/photos/" + owner + "/" + flickrID,
			GPSAltitude:     gpsAltitude,
			TerrainAltitude: terrainAltitude,
		})
	}

	return histogramEntry{
		Min:      minAlt,
		Max:      maxAlt,
		Count:    len(points),
		Examples: points[:min(20, len(points))],
	}, nil
}

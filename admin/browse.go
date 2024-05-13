package admin

import (
	"context"
	"github.com/jackc/pgx/v5"
	"log"
	"math"
	"net/http"
	"strconv"
)

func browseHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	regionParam := q.Get("r")
	pageParam := q.Get("p")

	var regionId *int
	if regionParam != "" {
		val, err := strconv.Atoi(regionParam)
		if err != nil {
			http.Error(w, "invalid region_id", http.StatusBadRequest)
			return
		}
		regionId = &val
	}

	var pageNum int
	if pageParam != "" {
		val, err := strconv.Atoi(pageParam)
		if err != nil {
			http.Error(w, "invalid page", http.StatusBadRequest)
			return
		}
		pageNum = val
	} else {
		pageNum = 1
	}

	hasNext, entries, err := loadBrowsePage(r.Context(), regionId, pageNum)
	if err != nil {
		log.Printf("error loading browse page: %s", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	pageCount, err := loadPageCount(r.Context(), regionId)
	if err != nil {
		log.Printf("error loading page count: %s", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	regions, err := listRegions(r.Context())
	if err != nil {
		log.Printf("error loading regions: %s", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	var regionName string
	if regionId != nil {
		for _, region := range regions {
			if region.ID == *regionId {
				regionName = region.Name
				break
			}
		}
	}

	var firstURL, prevURL, nextURL, lastURL string
	if pageNum > 2 {
		firstURL = "/browse?r=" + regionParam
	}
	if pageNum > 1 {
		prevURL = "/browse?r=" + regionParam + "&p=" + strconv.Itoa(pageNum-1)
	}
	if hasNext {
		nextURL = "/browse?r=" + regionParam + "&p=" + strconv.Itoa(pageNum+1)
	}
	if pageCount > pageNum+1 {
		lastURL = "/browse?r=" + regionParam + "&p=" + strconv.Itoa(pageCount)
	}

	templateResponse(w, r, "browse.tmpl.html", M{
		"Regions":    regions,
		"Page":       pageNum,
		"PageCount":  pageCount,
		"FirstURL":   firstURL,
		"PrevURL":    prevURL,
		"NextURL":    nextURL,
		"LastURL":    lastURL,
		"Region":     regionParam,
		"RegionName": regionName,
		"Entries":    entries,
	})
}

type browseEntry struct {
	Id           string
	RegionID     int
	RegionName   string
	MediumSource string
	HasGPS       bool
	OriginalURL  string
}

const perPage = 100

func loadBrowsePage(ctx context.Context, regionId *int, pageNum int) (bool, []browseEntry, error) {
	var rows pgx.Rows
	var err error
	if regionId == nil {
		rows, err = db.Query(ctx, `
			SELECT f.id, r.id, r.name, f.medium->>'source', f.info->'owner'->>'nsid',
				   coalesce(f.exif@>'[{"tag": "GPSLatitude"}]'::jsonb and f.exif@>'[{"tag": "GPSLongitude"}]'::jsonb, false)
			FROM flickr f
			JOIN regions r ON f.region_id = r.id
			ORDER BY f.inserted_at DESC
			LIMIT $1 OFFSET $2
	`, perPage, (pageNum-1)*perPage)
	} else {
		rows, err = db.Query(ctx, `
			SELECT f.id, r.id, r.name, f.medium->>'source', f.info->'owner'->>'nsid',
				   coalesce(f.exif@>'[{"tag": "GPSLatitude"}]'::jsonb and f.exif@>'[{"tag": "GPSLongitude"}]'::jsonb, false)
			FROM flickr f
			JOIN regions r ON f.region_id = r.id
			WHERE f.region_id = $1
			ORDER BY f.inserted_at DESC
			LIMIT $2 OFFSET $3
	`, regionId, perPage, (pageNum-1)*perPage)
	}
	if err != nil {
		return false, nil, err
	}
	defer rows.Close()

	var entries []browseEntry
	for rows.Next() {
		var entry browseEntry
		var owner string
		err = rows.Scan(&entry.Id, &entry.RegionID, &entry.RegionName, &entry.MediumSource, &owner, &entry.HasGPS)
		if err != nil {
			return false, nil, err
		}
		entry.OriginalURL = "https://www.flickr.com/photos/" + owner + "/" + entry.Id
		entries = append(entries, entry)
	}

	return len(entries) == perPage, entries, nil
}

func loadPageCount(ctx context.Context, regionId *int) (int, error) {
	var count int

	if regionId == nil {
		err := db.QueryRow(ctx, `
			SELECT COUNT(f.id)
			FROM flickr f
		`).Scan(&count)
		if err != nil {
			return 0, err
		}
	} else {
		err := db.QueryRow(ctx, `
		SELECT COUNT(f.id)
		FROM flickr f
		WHERE region_id = $1
	`, regionId).Scan(&count)
		if err != nil {
			return 0, err
		}
	}

	return int(math.Ceil(float64(count) / perPage)), nil
}

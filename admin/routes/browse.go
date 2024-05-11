package routes

import (
	"context"
	"encoding/base32"
	"encoding/binary"
	"math"
	"net/http"
	"strconv"
)

func browseHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	regionParam := q.Get("r")
	pageParam := q.Get("p")

	var internalRegionId *int
	if regionParam != "" {
		val, err := strconv.Atoi(regionParam)
		if err != nil {
			http.Error(w, "invalid region_id", http.StatusBadRequest)
			return
		}
		internalRegionId = &val
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

	var hasNext bool
	var pageCount int
	var entries []browseEntry
	if internalRegionId != nil {
		var err error
		hasNext, entries, err = loadBrowsePage(r.Context(), *internalRegionId, pageNum)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		pageCount, err = loadPageCount(r.Context(), *internalRegionId)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	regions, err := listRegions(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	var regionName string
	if internalRegionId != nil {
		for _, region := range regions {
			if region.ID == *internalRegionId {
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
	FlickrId          string
	ChallengeId       string
	PreviewURL        string
	ChallengeURL      string
	DebugChallengeURL string
	OriginalURL       string
	HasGPS            bool
}

const perPage = 50

func loadBrowsePage(ctx context.Context, regionId int, pageNum int) (bool, []browseEntry, error) {
	rows, err := Db.Query(ctx, `
		SELECT c.id, p.flickr_id, p.summary->>'server', p.summary->>'secret', p.summary->>'owner', p.exif->'GPSLatitude' is not null
		FROM challenges as c
		JOIN flickr_challenge_sources as fcs ON c.id = fcs.challenge_id
		JOIN flickr_photos as p ON fcs.flickr_id = p.flickr_id
		WHERE c.region_id = $1
		ORDER BY p.inserted_at DESC
		LIMIT $2 OFFSET $3
	`, regionId, perPage, (pageNum-1)*perPage)
	if err != nil {
		return false, nil, err
	}
	defer rows.Close()

	var entries []browseEntry
	for rows.Next() {
		var entry browseEntry
		var internalChallengeId int
		var owner string
		var server string
		var secret string
		err := rows.Scan(&internalChallengeId, &entry.FlickrId, &server, &secret, &owner, &entry.HasGPS)
		if err != nil {
			return false, nil, err
		}

		entry.PreviewURL = "https://live.staticflickr.com/" + server + "/" + entry.FlickrId + "_" + secret + "_m.jpg"
		entry.OriginalURL = "https://www.flickr.com/photos/" + owner + "/" + entry.FlickrId

		entry.ChallengeId = encodeChallengeID(internalChallengeId)
		entry.ChallengeURL = "https://contourguessr.org/c/" + entry.ChallengeId
		entry.DebugChallengeURL = "https://contourguessr.org/debug/c/" + entry.ChallengeId

		entries = append(entries, entry)
	}

	return len(entries) == perPage, entries, nil
}

func loadPageCount(ctx context.Context, regionId int) (int, error) {
	var count int
	err := Db.QueryRow(ctx, `
		SELECT COUNT(c.id)
		FROM challenges as c
		WHERE region_id = $1
	`, regionId).Scan(&count)
	if err != nil {
		return 0, err
	}
	return int(math.Ceil(float64(count) / perPage)), nil
}

var encoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)
var endianness = binary.BigEndian

func encodeChallengeID(id int) string {
	if id <= 0 || id >= 0xFFFFFFFF {
		panic("id out of bounds")
	}

	bytes := make([]byte, 4)
	endianness.PutUint32(bytes[:], uint32(id))

	for bytes[0] == 0 {
		bytes = bytes[1:]
	}

	return encoding.EncodeToString(bytes[:])
}

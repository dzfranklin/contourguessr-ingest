package admin

import (
	"context"
	"net/http"
	"time"
)

func overviewHandler(w http.ResponseWriter, r *http.Request) {
	ingestCounts, err := loadIngestCounts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalCount := 0
	for _, count := range ingestCounts {
		totalCount += count.Count
	}

	templateResponse(w, r, "overview.tmpl.html", M{
		"TotalCount":   totalCount,
		"IngestCounts": ingestCounts,
	})
}

type loadIngestEntry struct {
	RegionID   int
	RegionName string
	Count      int
	Latest     time.Time
	LastCheck  *time.Time
}

func loadIngestCounts(ctx context.Context) ([]loadIngestEntry, error) {
	rows, err := db.Query(ctx, `
		SELECT r.id, r.name, count(f.id),
		       max(to_timestamp((f.info ->> 'dateuploaded')::int))::date, max(c.last_check)
		FROM regions r
				 JOIN region_index_cursors c on r.id = c.region_id
				 LEFT JOIN flickr f on f.region_id = r.id
		GROUP BY r.name, r.id
		ORDER BY r.name, r.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []loadIngestEntry
	for rows.Next() {
		var count loadIngestEntry
		err = rows.Scan(&count.RegionID, &count.RegionName, &count.Count,
			&count.Latest, &count.LastCheck)
		if err != nil {
			return nil, err
		}
		counts = append(counts, count)
	}

	return counts, nil
}

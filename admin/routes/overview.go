package routes

import (
	"context"
	"net/http"
)

func overviewHandler(w http.ResponseWriter, r *http.Request) {
	counts, err := loadCounts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templateResponse(w, r, "overview.tmpl.html", M{
		"Counts": counts,
	})
}

type regionCountsEntry struct {
	RegionID int
	Name     string
	Indexed  int
	Scored   int
	Accepted int
}

func loadCounts(ctx context.Context) ([]regionCountsEntry, error) {
	rows, err := Db.Query(ctx, `
		SELECT r.id,
			   r.name,
			   count(p.flickr_id)                                as count_indexed,
			   count(p.flickr_id) filter ( where s.is_complete ) as count_scored,
			   count(p.flickr_id) filter ( where s.is_accepted)  as count_accepted
		FROM flickr_photos as p
				 LEFT JOIN photo_scores as s ON s.flickr_photo_id = p.flickr_id AND s.vsn = (SELECT max(vsn)
																							 FROM photo_scores)
				 RIGHT JOIN regions as r ON p.region_id = r.id
		GROUP BY r.id, r.name
		ORDER BY r.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []regionCountsEntry
	for rows.Next() {
		var count regionCountsEntry
		err = rows.Scan(&count.RegionID, &count.Name, &count.Indexed, &count.Scored, &count.Accepted)
		if err != nil {
			return nil, err
		}
		counts = append(counts, count)
	}

	return counts, nil
}

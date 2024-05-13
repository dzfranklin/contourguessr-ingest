package repos

import (
	"context"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/encoding/ewkb"
)

type Region struct {
	Id   int
	Name string
	Geo  orb.MultiPolygon
}

func (r *Repo) GetRegion(ctx context.Context, id int) (Region, error) {
	var region Region
	err := r.db.QueryRow(ctx, `
		SELECT id, name, ST_AsBinary(geo::geometry)
		FROM regions
		WHERE id = $1
`, id).Scan(&region.Id, &region.Name, ewkb.Scanner(&region.Geo))
	if err != nil {
		return Region{}, err
	}
	return region, nil
}

func (r *Repo) ListRegions(ctx context.Context) ([]Region, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, ST_AsBinary(geo::geometry)
		FROM regions
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var regions []Region
	for rows.Next() {
		var r Region
		if err := rows.Scan(&r.Id, &r.Name, ewkb.Scanner(&r.Geo)); err != nil {
			return nil, err
		}
		regions = append(regions, r)
	}

	return regions, nil
}

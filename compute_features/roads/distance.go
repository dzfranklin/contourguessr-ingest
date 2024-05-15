package roads

import (
	"context"
	"contourguessr-ingest/overpass"
	_ "embed"
	"fmt"
	"github.com/golang/geo/s2"
	"html/template"
	"math"
	"strings"
)

// The Earth's mean radius in kilometers
const earthRadiusKm = 6371.01

type Overpass interface {
	Query(ctx context.Context, query string) (*overpass.Response, error)
}

//go:embed distance.tmpl
var distanceTmplText string
var distanceTmpl = template.Must(template.New("distance").Parse(distanceTmplText))

// DistanceToRoad returns the distance in meters from the given point to the nearest paved road.
//
// The result is clamped to 1000 meters.
func DistanceToRoad(ctx context.Context, ovp Overpass, lng, lat float64) (int, error) {
	var sb strings.Builder
	err := distanceTmpl.Execute(&sb, struct {
		Lng float64
		Lat float64
	}{
		Lng: lng,
		Lat: lat,
	})
	if err != nil {
		return 0, fmt.Errorf("execute template: %w", err)
	}

	resp, err := ovp.Query(ctx, sb.String())
	if err != nil {
		return 0, fmt.Errorf("query pverpass: %w", err)
	}

	if len(resp.Ways) == 0 {
		return 1000, nil
	}

	// This algorithm is naive but querying overpass dominates

	closest := 1000
	p := s2.PointFromLatLng(s2.LatLngFromDegrees(lat, lng))
	for _, way := range resp.Ways {
		if !isRoad(way.Tags["highway"], way.Tags["surface"]) {
			continue
		}

		for i := 0; i < len(way.Nodes)-2; i++ {
			a := s2.PointFromLatLng(s2.LatLngFromDegrees(way.Nodes[i].Lat, way.Nodes[i].Lon))
			b := s2.PointFromLatLng(s2.LatLngFromDegrees(way.Nodes[i+1].Lat, way.Nodes[i+1].Lon))
			angle := s2.DistanceFromSegment(p, a, b)
			dist := int(math.Round(earthRadiusKm * float64(angle) * 1000))
			if dist < closest {
				closest = dist
			}
		}
	}

	return closest, nil
}

func isRoad(highway, surface string) bool {
	// If paved then it is a road
	if tagValueContains(surface,
		"paved",    // A feature that is predominantly paved; i.e., it is covered with paving stones, concrete or bitumen
		"asphalt",  // Short for asphalt concrete
		"chipseal", // Less expensive alternative to asphalt concrete. Rarely tagged
		"concrete", // Portland cement concrete, forming a large surface
	) {
		return true
	}

	// If the highway value matches the denylist then assume it is a road. There is a long tail of weird tags that
	// we err on the side of assuming aren't roads.
	if tagValueContains(highway,
		// From OSM wiki
		"motorway",      // A restricted access major divided highway,
		"trunk",         // The most important roads in a country's system that aren't motorways
		"primary",       // After trunk
		"secondary",     // After primary
		"tertiary",      // After secondary
		"unclassified",  // The least important through roads in a country's system. The word 'unclassified' is a historical artefact of the UK road system and does not mean that the classification is unknown
		"residential",   // Roads which serve as access to housing, without function of connecting settlements
		"motorway_link", // The link roads (sliproads/ramps) leading to/from a motorway from/to a motorway or lower class highway
		"trunk_link",
		"primary_link",
		"secondary_link",
		"tertiary_link",
		"living_street", // residential streets where pedestrians have legal priority over cars
		"service",       // For access roads to, or within an industrial estate, camp site, business park, car park, alleys, etc
		"raceway",       // A course or track for (motor) racing
		"busway",        // Dedicated roadway for buses
		"rest_area",
	) {
		return true
	}

	return false
}

func tagValueContains(tagValue string, needles ...string) bool {
	values := strings.Split(tagValue, ";")
	for _, v := range values {
		for _, n := range needles {
			if v == n {
				return true
			}
			if strings.HasPrefix(v, n+":") {
				return true
			}
		}
	}
	return false
}

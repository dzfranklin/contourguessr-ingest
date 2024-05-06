package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func queryRoadWithin1000m(lng, lat float64) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	reqBody := strings.NewReader(fmt.Sprintf(`
		[out:json];
		wr(around:1000,%0.6f,%0.6f)[highway];
		out tags;
	`, lat, lng))

	req, err := http.NewRequestWithContext(ctx, "POST", overpassEndpoint+"/interpreter", reqBody)
	if err != nil {
		return false, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var ovpResp struct {
		Elements []struct {
			Tags map[string]string `json:"tags"`
		} `json:"elements"`
	}
	err = json.NewDecoder(resp.Body).Decode(&ovpResp)
	if err != nil {
		return false, err
	}

	for _, elem := range ovpResp.Elements {
		highway := elem.Tags["highway"]
		surface := elem.Tags["surface"]

		// If paved then it is a road
		if tagValueContains(surface,
			"paved",    // A feature that is predominantly paved; i.e., it is covered with paving stones, concrete or bitumen
			"asphalt",  // Short for asphalt concrete
			"chipseal", // Less expensive alternative to asphalt concrete. Rarely tagged
			"concrete", // Portland cement concrete, forming a large surface
		) {
			return true, nil
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
			// From our examples
		) {
			return true, nil
		}
	}

	return false, nil
}

func tagValueContains(tagValue string, needles ...string) bool {
	values := strings.Split(tagValue, ";")
	for _, v := range values {
		for _, n := range needles {
			if strings.Contains(v, n) {
				return true
			}
			if strings.HasPrefix(v, n+":") {
				return true
			}
		}
	}
	return false
}

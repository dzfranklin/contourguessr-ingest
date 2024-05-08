package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

func getElevation(lng, lat float64) (float64, error) {
	u, err := url.Parse("http://dev.virtualearth.net/REST/v1/Elevation/List")
	if err != nil {
		return 0, err
	}

	q := u.Query()
	q.Add("heights", "ellipsoid")
	q.Add("points", fmt.Sprintf("%f,%f", lat, lng))
	q.Add("key", bingMapsKey)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return 0, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected http status %d", resp.StatusCode)
	}

	var respData struct {
		ResourceSets []struct {
			Resources []struct {
				Elevations []float64
			}
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return 0, err
	}

	if len(respData.ResourceSets) != 1 {
		return 0, fmt.Errorf("unexpected number of ResourceSets: %d", len(respData.ResourceSets))
	}
	if len(respData.ResourceSets[0].Resources) != 1 {
		return 0, fmt.Errorf("unexpected number of Resources: %d", len(respData.ResourceSets[0].Resources))
	}
	if len(respData.ResourceSets[0].Resources[0].Elevations) != 1 {
		return 0, fmt.Errorf("unexpected number of Elevations: %d", len(respData.ResourceSets[0].Resources[0].Elevations))
	}
	value := respData.ResourceSets[0].Resources[0].Elevations[0]

	return value, nil
}

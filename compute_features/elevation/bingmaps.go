package elevation

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cenkalti/backoff/v4"
	"net/http"
	"net/url"
	"time"
)

type BingMaps struct {
	key  string
	http *http.Client
}

func NewBingMaps(key string) *BingMaps {
	return &BingMaps{key: key, http: &http.Client{}}
}

func (c *BingMaps) LookupElevation(ctx context.Context, lng, lat float64) (float64, error) {
	u, err := url.Parse("http://dev.virtualearth.net/REST/v1/Elevation/List")
	if err != nil {
		return 0, err
	}

	q := u.Query()
	q.Add("heights", "ellipsoid")
	q.Add("points", fmt.Sprintf("%f,%f", lat, lng))
	q.Add("key", c.key)
	u.RawQuery = q.Encode()

	var value float64
	err = backoff.Retry(func() error {
		req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
		if err != nil {
			return err
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("unexpected http status %d", resp.StatusCode)
			if resp.StatusCode >= 500 {
				return err
			} else {
				return backoff.Permanent(err)
			}
		}

		var respData struct {
			ResourceSets []struct {
				Resources []struct {
					Elevations []float64
				}
			}
		}
		if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
			return err
		}

		if len(respData.ResourceSets) != 1 {
			return fmt.Errorf("unexpected number of ResourceSets: %d", len(respData.ResourceSets))
		}
		if len(respData.ResourceSets[0].Resources) != 1 {
			return fmt.Errorf("unexpected number of Resources: %d", len(respData.ResourceSets[0].Resources))
		}
		if len(respData.ResourceSets[0].Resources[0].Elevations) != 1 {
			return fmt.Errorf("unexpected number of Elevations: %d", len(respData.ResourceSets[0].Resources[0].Elevations))
		}
		value = respData.ResourceSets[0].Resources[0].Elevations[0]

		return nil
	}, backoff.NewExponentialBackOff(backoff.WithMaxElapsedTime(10*time.Second)))
	return value, err
}

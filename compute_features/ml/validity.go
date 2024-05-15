package ml

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/cenkalti/backoff/v4"
	"log/slog"
	"net/http"
)

type ValidityResult struct {
	Score float64 `json:"validity_score"`
	Model string  `json:"model"`
}

func (c *Client) PredictValidity(ctx context.Context, photoData []byte) (*ValidityResult, error) {
	var result *ValidityResult
	err := backoff.Retry(func() error {
		var err error
		result, err = c.queryValidityNoRetry(ctx, photoData)
		if err != nil {
			slog.Warn("Error querying validity", "err", err)
		}
		return err
	}, backoff.NewExponentialBackOff())
	return result, err
}

func (c *Client) queryValidityNoRetry(ctx context.Context, photoData []byte) (*ValidityResult, error) {
	reqData := struct {
		ImageBase64 string `json:"image_base64"`
	}{base64.StdEncoding.EncodeToString(photoData)}
	reqBody, err := json.Marshal(reqData)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/api/v0/classify", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result ValidityResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

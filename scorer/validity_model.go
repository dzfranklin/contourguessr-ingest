package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/cenkalti/backoff/v4"
	"log"
	"net/http"
	"time"
)

type validityResult struct {
	Score float64 `json:"validity_score"`
	Model string  `json:"model"`
}

func queryValidity(photoData []byte) (*validityResult, error) {
	var result *validityResult
	err := backoff.Retry(func() error {
		var err error
		result, err = queryValidityNoRetry(photoData)
		return err
	}, backoff.NewExponentialBackOff())
	if err != nil {
		log.Printf("Error querying validity: %v", err)
	}
	return result, err
}

func queryValidityNoRetry(photoData []byte) (*validityResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reqData := struct {
		ImageBase64 string `json:"image_base64"`
	}{base64.StdEncoding.EncodeToString(photoData)}
	reqBody, err := json.Marshal(reqData)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", classifierEndpoint+"/api/v0/classify", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result validityResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

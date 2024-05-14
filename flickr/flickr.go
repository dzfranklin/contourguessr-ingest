package flickr

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cenkalti/backoff/v4"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"
)

type SearchResponse struct {
	Page    int `json:"page"`
	Pages   int `json:"pages"`
	Perpage int `json:"perpage"`
	Total   int `json:"total"`
	Photos  struct {
		Page    int     `json:"page"`
		Pages   int     `json:"pages"`
		PerPage int     `json:"perpage"`
		Total   int     `json:"total"`
		Photo   []Photo `json:"photo"`
	} `json:"photos"`
}

type Photo struct {
	ID     string `json:"id"`
	Owner  string `json:"owner"`
	Secret string `json:"secret"`
	Server string `json:"server"`
	Title  string `json:"title"`

	// only if you requested the extras
	DateUpload string `json:"dateupload"`
	Latitude   string `json:"latitude"`
	Longitude  string `json:"longitude"`
	Accuracy   string `json:"accuracy"`
}

type Client struct {
	APIKey    string
	Endpoint  *url.URL
	SkipWaits bool
}

func NewFromEnv() *Client {
	apiKey := os.Getenv("FLICKR_API_KEY")
	if apiKey == "" {
		log.Fatal("FLICKR_API_KEY not set")
	}

	endpoint := os.Getenv("FLICKR_ENDPOINT")
	if endpoint == "" {
		log.Fatal("FLICKR_ENDPOINT not set")
	}

	fc, err := New(apiKey, endpoint)
	if err != nil {
		log.Fatal(err)
	}
	return fc
}

func New(apiKey string, endpoint string) (*Client, error) {
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}
	return &Client{
		APIKey:   apiKey,
		Endpoint: endpointURL,
	}, nil
}

func (f *Client) Download(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "github.com/dzfranklin/contourguessr-ingest")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

var lastCall time.Time
var callMu sync.Mutex

func (f *Client) Call(ctx context.Context, method string, resp any, params map[string]string) error {
	if !f.SkipWaits {
		callMu.Lock()
		defer callMu.Unlock()
		time.Sleep(time.Until(lastCall.Add(1 * time.Second)))
		lastCall = time.Now()
	}

	if params == nil {
		params = make(map[string]string)
	}
	params["method"] = method
	params["format"] = "json"
	params["nojsoncallback"] = "1"

	query := url.Values{}
	for k, v := range params {
		query.Set(k, v)
	}

	r := *f.Endpoint
	r.Path = "/services/rest"
	r.RawQuery = query.Encode()
	u := r.String()

	return backoff.Retry(func() error {
		slog.Debug("flickr call", "method", method, "url", u)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-Api-Key", f.APIKey)

		httpResp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		if httpResp.StatusCode != http.StatusOK {
			return fmt.Errorf("HTTP status %d", httpResp.StatusCode)
		}

		defer httpResp.Body.Close()

		body, err := io.ReadAll(httpResp.Body)
		if err != nil {
			return err
		}

		err = json.Unmarshal(body, &resp)
		if err != nil {
			return err
		}

		return nil
	}, backoff.NewExponentialBackOff())
}

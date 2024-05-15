package overpass

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	endpoint string
	http     *http.Client
}

func New(endpoint string) *Client {
	return &Client{endpoint: endpoint, http: &http.Client{}}
}

type QueryError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *QueryError) Error() string {
	return fmt.Sprintf("query error: %d %s: %s", e.StatusCode, e.Status, e.Body)
}

func (c *Client) Query(ctx context.Context, query string) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, strings.NewReader(query))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "github.com/dzfranklin/contourguessr-ingest")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			body = nil
		}
		return nil, &QueryError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       string(body),
		}
	}
	return ParseJSON(resp.Body)
}

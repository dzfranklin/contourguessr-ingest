package roads

import (
	"context"
	"contourguessr-ingest/overpass"
	_ "embed"
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

//go:embed test_example1_ovp.json
var testOverpassResponse string

func TestDistanceToRoad(t *testing.T) {
	ctx := context.Background()

	// Calculated in QGIS via a GPX export from Overpass Turbo and manual use of the ruler tool
	exp := 315

	ovp := mockOverpass(t, `[out:json];
way(around:1000,39.778578,-105.494735)["highway"];
(._;>;);
out;
`, testOverpassResponse, nil)
	dist, err := DistanceToRoad(ctx, ovp, -105.494735, 39.778578)
	assert.Nil(t, err)

	assert.InDelta(t, exp, dist, 10)
}

//go:embed test_bug1_ovp.json
var testBug1Ovp string

func TestDistanceToRoadBug1(t *testing.T) {
	ctx := context.Background()

	ovp := mockOverpass(t, `[out:json];
way(around:1000,36.348733,-117.068628)["highway"];
(._;>;);
out;
`, testBug1Ovp, nil)
	dist, err := DistanceToRoad(ctx, ovp, -117.068628, 36.348733)
	assert.Nil(t, err)

	assert.Greater(t, dist, 100)
}

func BenchmarkDistanceToRoad(b *testing.B) {
	ctx := context.Background()
	ovp := mockOverpass(nil, `[out:json];
way(around:1000,39.778578,-105.494735)["highway"];
(._;>;);
out;
`, testOverpassResponse, nil)

	for i := 0; i < b.N; i++ {
		_, err := DistanceToRoad(ctx, ovp, -105.494735, 39.778578)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func mockOverpass(t *testing.T, query, resp string, err error) *mockOverpassClient {
	return &mockOverpassClient{
		t:     t,
		query: query,
		resp:  resp,
		err:   err,
	}
}

type mockOverpassClient struct {
	t     *testing.T
	query string
	resp  string
	err   error
}

func (m *mockOverpassClient) Query(_ context.Context, query string) (*overpass.Response, error) {
	if m.t == nil {
		if query != m.query {
			panic("unexpected query")
		}
	} else {
		assert.Equal(m.t, m.query, query)
	}

	resp, err := overpass.ParseJSON(strings.NewReader(m.resp))
	if err != nil {
		if m.t == nil {
			panic(err)
		} else {
			m.t.Fatalf("failed to parse mock response: %v", err)
		}
	}

	return resp, m.err
}

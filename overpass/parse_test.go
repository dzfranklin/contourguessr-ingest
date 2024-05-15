package overpass

import (
	_ "embed"
	"github.com/bradleyjkemp/cupaloy"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

//go:embed sample.json
var testSample string

func TestParseSample(t *testing.T) {
	sample := strings.NewReader(testSample)
	resp, err := ParseJSON(sample)
	require.Nil(t, err)
	cupaloy.SnapshotT(t, resp)
}

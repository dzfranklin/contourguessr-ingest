package overpass

import (
	"encoding/json"
	"io"
)

type rawResponse struct {
	Generator string
	Elements  []rawElement
}

type rawElement struct {
	Type string

	// meta
	ID   int64
	Tags map[string]string

	// node
	Lat float64
	Lon float64

	// way
	Nodes []int64
}

func ParseJSON(v io.Reader) (*Response, error) {
	var resp rawResponse
	err := json.NewDecoder(v).Decode(&resp)
	if err != nil {
		return nil, err
	}

	response := &Response{
		Generator: resp.Generator,
		Count:     len(resp.Elements),
		Nodes:     make(map[int64]*Node),
		Ways:      make(map[int64]*Way),
	}

	for _, el := range resp.Elements {
		switch el.Type {
		case "node":
			response.Nodes[el.ID] = &Node{
				Meta: Meta{
					ID:   el.ID,
					Tags: el.Tags,
				},
				Lat: el.Lat,
				Lon: el.Lon,
			}
		case "way":
			way := &Way{
				Meta: Meta{
					ID:   el.ID,
					Tags: el.Tags,
				},
				Nodes: make([]*Node, len(el.Nodes)),
			}
			for i, nodeID := range el.Nodes {
				way.Nodes[i] = response.Nodes[nodeID]
			}
			response.Ways[el.ID] = way
		}
	}

	return response, nil
}

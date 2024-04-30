package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v4"
	"github.com/joho/godotenv"
	flag "github.com/spf13/pflag"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

var query = `
[out:json];

way({{bbox_1}},{{bbox_0}},{{bbox_3}},{{bbox_2}})
  [highway~"^(residential|unclassified|primary|trunk|tertiary|secondary|living_street|raceway|primary_link|rest_area|tertiary_link|road|services)$"];

( ._; >; );

out;
`

var badHighways = []string{
	"service",
	"residential",
	"unclassified",
	"primary",
	"trunk",
	"tertiary",
	"secondary",
	"living_street",
	"raceway",
	"primary_link",
	"rest_area",
	"tertiary_link",
	"road",
	"services",
}

/*
CREATE INDEX roads_line_geom_idx ON roads USING GIST (line_geom);
*/

/*
SELECT 100 * (SELECT count(DISTINCT p.id)
        FROM flickr_photos as p
                 LEFT JOIN roads as r
                           ON ST_DWithin(p.geom, r.line_geom, 400)
        WHERE r IS NULL AND p.region = 'uk_lake_district') /
       (SELECT count(p.id)
        FROM flickr_photos as p WHERE p.region = 'uk_lake_district') as pc_good;
*/

var regionFilter = flag.StringP("region", "r", "", "Prefix of region IDs to ingest")

func init() {
	err := godotenv.Load(".env", ".env.local")
	if err != nil {
		log.Println(err)
	}

	flag.Parse()
}

func main() {
	ctx := context.Background()

	db, err := pgx.Connect(ctx, os.Getenv("INGEST_DB"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close(ctx)
	err = db.Ping(ctx)
	if err != nil {
		log.Fatal("failed to connect to database: ", err)
	}
	_, err = db.Exec(ctx, "DROP TABLE IF EXISTS roads")
	if err != nil {
		log.Fatal(err)
	}
	_, err = db.Exec(ctx, `CREATE TABLE roads (
    	id SERIAL PRIMARY KEY,
    	osm_id TEXT,
    	line_geom GEOGRAPHY
	)`)
	if err != nil {
		log.Fatal(err)
	}

	regionsFile, err := os.Open("regions.json")
	if err != nil {
		log.Fatal(err)
	}
	defer regionsFile.Close()
	dec := json.NewDecoder(regionsFile)
	var regions map[string]string
	if err := dec.Decode(&regions); err != nil {
		log.Fatal(err)
	}

	for region, bbox := range regions {
		if *regionFilter != "" && !strings.HasPrefix(region, *regionFilter) {
			log.Printf("Skipping %s", region)
			continue
		}

		log.Printf("Processing %s", region)

		parts := strings.Split(bbox, ",")
		q := strings.ReplaceAll(query, "{{bbox_0}}", parts[0])
		q = strings.ReplaceAll(q, "{{bbox_1}}", parts[1])
		q = strings.ReplaceAll(q, "{{bbox_2}}", parts[2])
		q = strings.ReplaceAll(q, "{{bbox_3}}", parts[3])

		data := queryOverpass(q)

		nodes := make(map[int]overpassElement)
		ways := make(map[int]overpassElement)
		for _, elem := range data.Elements {
			switch elem.Type {
			case "node":
				nodes[elem.ID] = elem
			case "way":
				ways[elem.ID] = elem
			}
		}

		var batch pgx.Batch

		for _, way := range ways {
			if !contains(badHighways, way.Tags.Highway) {
				continue
			}

			points := make([]string, 0, len(way.Nodes))
			for _, nodeID := range way.Nodes {
				node, ok := nodes[nodeID]
				if !ok {
					log.Printf("Missing node %d", nodeID)
					continue
				}
				points = append(points, fmt.Sprintf("ST_Point(%f,%f,4326)", node.Lon, node.Lat))
			}

			batch.Queue(fmt.Sprintf("INSERT INTO roads (osm_id, line_geom) VALUES (%d, ST_MakeLine(ARRAY[%s])::geography)",
				way.ID, strings.Join(points, ",")))
		}

		results := db.SendBatch(ctx, &batch)
		for i := 0; i < batch.Len(); i++ {
			if _, err := results.Exec(); err != nil {
				log.Fatal(err)
			}
		}
		if err := results.Close(); err != nil {
			log.Fatal(err)
		}
		log.Printf("Inserted %d roads", batch.Len())
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

type overpassResponse struct {
	Elements []overpassElement `json:"elements"`
}

type overpassElement struct {
	Type string `json:"type"`
	ID   int    `json:"id"`

	Tags struct {
		Highway string `json:"highway"`
	}

	// type: node
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`

	// type: way
	Nodes []int `json:"nodes"`
}

func queryOverpass(query string) overpassResponse {
	req, err := http.NewRequest("GET", "https://overpass-api.de/api/interpreter?data="+url.QueryEscape(query), nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("User-Agent", "contourguessr.org (contact daniel@danielzfranklin.org)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("HTTP status %d", resp.StatusCode)
	}

	var data overpassResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Fatal(err)
	}

	return data
}

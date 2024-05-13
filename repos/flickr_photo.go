package repos

import (
	"encoding/json"
	"time"
)

type FlickrPhoto struct {
	Id         string          `json:"id"`
	RegionId   int             `json:"region_id"`
	InsertedAt *time.Time      `json:"inserted_at"`
	Info       json.RawMessage `json:"info"`
	Sizes      json.RawMessage `json:"sizes"`
	Exif       json.RawMessage `json:"exif"`

	Medium PhotoSize `json:"medium"` // corresponds to no suffix, 500px longest edge
	Large  PhotoSize `json:"large"`  // largest available size besides original
}

type PhotoSize struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Source string `json:"source"`
}

package main

import (
	"log"
	"regexp"
	"strconv"
)

var altitudeRe = regexp.MustCompile(`^(\d+(?:\.\d+)?) m$`)

func exifGPSAltitude(exif map[string]string) (float64, bool) {
	valS, ok := exif["GPSAltitude"]
	if !ok {
		return 0, false
	}

	ref, ok := exif["GPSAltitudeRef"]
	if !ok {
		return 0, false
	}

	valGroups := altitudeRe.FindStringSubmatch(valS)
	if valGroups == nil {
		log.Printf("Unexpected GPSAltitude (regex does not match): %s", valS)
		return 0, false
	}
	val, err := strconv.ParseFloat(valGroups[1], 64)
	if err != nil {
		log.Printf("Unexpected GPSAltitude (not a float): %s", valS)
		return 0, false
	}

	switch ref {
	case "Above Sea Level":
		return val, true
	case "Below Sea Level":
		return -val, true
	default:
		log.Printf("Unexpected GPSAltitudeRef: %s", ref)
		return 0, false
	}
}

package elevation

import (
	"contourguessr-ingest/repos"
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

var altitudeRe = regexp.MustCompile(`^(\d+(?:\.\d+)?) m$`)

func ParseGPSAltitude(exif repos.Exif) (float64, error) {
	valS, ok := exif.GetRaw("GPSAltitude")
	if !ok {
		return 0, errors.New("no GPSAltitude tag")
	}

	ref, ok := exif.GetRaw("GPSAltitudeRef")
	if !ok {
		return 0, errors.New("no GPSAltitudeRef tag")
	}

	valGroups := altitudeRe.FindStringSubmatch(valS)
	if valGroups == nil {
		return 0, fmt.Errorf("unexpected GPSAltitude (regex does not match): %s", valS)
	}
	val, err := strconv.ParseFloat(valGroups[1], 64)
	if err != nil {
		return 0, fmt.Errorf("unexpected GPSAltitude (not a float): %s", valS)
	}

	switch ref {
	case "Above Sea Level":
		return val, nil
	case "Below Sea Level":
		return -val, nil
	default:
		return 0, fmt.Errorf("unexpected GPSAltitudeRef: %s", ref)
	}
}

package flickr

import (
	"strconv"
	"time"
)

type ParseTimeError struct {
	s string
}

func (e *ParseTimeError) Error() string {
	return "invalid time: " + e.s
}

func ParseTime(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err == nil {
		return t, nil
	}

	secs, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, &ParseTimeError{s}
	}
	return time.Unix(secs, 0), nil
}

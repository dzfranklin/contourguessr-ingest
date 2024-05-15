package admin

import (
	"contourguessr-ingest/repos"
	"net/http"
	"reflect"
	"slices"
	"strings"
)

func infoHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	photo, err := repo.GetPhoto(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	features, err := repo.GetFeatures(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	region, err := repo.GetRegion(r.Context(), photo.RegionId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	lng, lat, err := photo.ParseLngLat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templateResponse(w, r, "info.tmpl.html", M{
		"Photo":          photo,
		"FeatureList":    featureList(features),
		"Region":         region,
		"MaptilerAPIKey": mtApiKey,
		"Lng":            lng,
		"Lat":            lat,
	})
}

type featureEntry struct {
	Name  string
	Value any
}

func featureList(features repos.Features) []featureEntry {
	v := reflect.ValueOf(features)
	var out []featureEntry
	for i := 0; i < v.NumField(); i++ {
		out = append(out, featureEntry{
			Name:  v.Type().Field(i).Name,
			Value: v.Field(i).Interface(),
		})
	}
	slices.SortFunc(out, func(i, j featureEntry) int {
		return strings.Compare(i.Name, j.Name)
	})
	return out
}

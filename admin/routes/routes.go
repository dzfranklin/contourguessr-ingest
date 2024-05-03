package routes

import (
	"context"
	"embed"
	"github.com/jackc/pgx/v4/pgxpool"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
)

var Db *pgxpool.Pool
var MaptilerAPIKey string

//go:embed *.tmpl.*
var templateFS embed.FS

var routeTemplates map[string]*template.Template
var appEnv string
var navEntries []navEntry

type M map[string]interface{}

type navEntry struct {
	Path      string
	Title     string
	IsCurrent bool
}

func init() {
	appEnv = os.Getenv("APP_ENV")

	routeTemplates = make(map[string]*template.Template)
	entries, err := templateFS.ReadDir(".")
	if err != nil {
		log.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == "layout.tmpl.html" ||
			!strings.HasSuffix(name, ".tmpl.html") {
			continue
		}
		tmpl, err := template.ParseFS(templateFS, "layout.tmpl.html", name)
		if err != nil {
			log.Fatalf("Error parsing template %s with layout: %v", name, err)
		}
		routeTemplates[name] = tmpl
	}

	navEntries = []navEntry{
		{Path: "/", Title: "Home"},
		{Path: "/plot", Title: "Plot"},
	}

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/plot", plotHandler)
}

func templateResponse(w http.ResponseWriter, r *http.Request, name string, data M) {
	if data == nil {
		data = make(M)
	}

	var err error
	var tmpl *template.Template

	if appEnv == "development" {
		log.Println("Loading templates directly (dev mode)")
		tmpl, err = template.ParseFiles("admin/routes/layout.tmpl.html", "admin/routes/"+name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		tmpl = routeTemplates[name]
		if tmpl == nil {
			http.Error(w, "Template not found", http.StatusInternalServerError)
			return
		}
	}

	var responseNavEntries []navEntry
	for _, entry := range navEntries {
		responseNavEntries = append(responseNavEntries, entry)
		if normalizeNavEntryPath(entry.Path) == normalizeNavEntryPath(r.URL.Path) {
			responseNavEntries[len(responseNavEntries)-1].IsCurrent = true
		}
	}
	data["NavEntries"] = responseNavEntries

	err = tmpl.Execute(w, data)
	if err != nil {
		if appEnv == "development" {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

func normalizeNavEntryPath(path string) string {
	for strings.HasPrefix(path, "/") {
		path = strings.TrimPrefix(path, "/")
	}
	for strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}

type regionListEntry struct {
	ID   int
	Name string
}

func listRegions(ctx context.Context) ([]regionListEntry, error) {
	rows, err := Db.Query(ctx, `SELECT id, name FROM regions ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var regions []regionListEntry
	for rows.Next() {
		var region regionListEntry
		err = rows.Scan(&region.ID, &region.Name)
		if err != nil {
			return nil, err
		}
		regions = append(regions, region)
	}

	return regions, nil
}

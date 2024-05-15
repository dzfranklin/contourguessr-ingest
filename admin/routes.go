package admin

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

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

var funcMap = template.FuncMap{
	"percent": func(numerator, denominator int) string {
		if denominator == 0 {
			return "0.00%"
		}
		return fmt.Sprintf("%.2f%%", float64(numerator)/float64(denominator)*100)
	},
	"iecBytes": func(bytes int64) string {
		if bytes < 1024 {
			return fmt.Sprintf("%d B", bytes)
		} else if bytes < 1024*1024 {
			return fmt.Sprintf("%.2f KiB", float64(bytes)/1024)
		} else if bytes < 1024*1024*1024 {
			return fmt.Sprintf("%.2f MiB", float64(bytes)/1024/1024)
		} else {
			return fmt.Sprintf("%.2f GiB", float64(bytes)/1024/1024/1024)
		}
	},
	"score": func(score float64) string {
		return fmt.Sprintf("%.2f", score)
	},
	"prettyJSON": func(v interface{}) string {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Sprintf("marshal: %v", err)
		}
		return string(b)
	},
	"sub": func(a, b int) int {
		return a - b
	},
}

func Mux() http.Handler {
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
		tmpl, err := prepareEmptyTemplate(name).ParseFS(templateFS, "layout.tmpl.html", name)
		if err != nil {
			log.Fatalf("Error parsing template %s with layout: %v", name, err)
		}
		tmpl.Funcs(funcMap)
		routeTemplates[name] = tmpl
	}

	navEntries = []navEntry{
		{Path: "/", Title: "Home"},
		{Path: "/overview", Title: "Overview"},
		{Path: "/storage", Title: "Storage"},
		{Path: "/browse", Title: "Browse"},
		{Path: "/plot", Title: "Plot"},
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", indexHandler)
	mux.HandleFunc("/overview", overviewHandler)
	mux.HandleFunc("/storage", storageHandler)
	mux.HandleFunc("/plot", plotHandler)
	mux.HandleFunc("/browse", browseHandler)
	mux.HandleFunc("/info/{id}", infoHandler)

	return timingMiddleware(mux)
}

func templateResponse(w http.ResponseWriter, r *http.Request, name string, data M) {
	if data == nil {
		data = make(M)
	}

	var err error
	var tmpl *template.Template

	if appEnv == "development" {
		slog.Warn("Loading templates directly (dev mode)")
		tmpl, err = prepareEmptyTemplate(name).ParseFiles("admin/layout.tmpl.html", "admin/"+name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.Funcs(funcMap)
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
	data["LayoutNavEntries"] = responseNavEntries

	err = tmpl.Execute(w, data)
	if err != nil {
		if appEnv == "development" {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

func prepareEmptyTemplate(name string) *template.Template {
	tmpl := template.New(name)
	tmpl.Funcs(funcMap)
	return tmpl
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
	rows, err := db.Query(ctx, `SELECT id, name FROM regions ORDER BY name`)
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

func timingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		total := time.Since(start)

		type valueType struct {
			TotalText string `json:"total_text"`
		}
		value := valueType{
			TotalText: fmt.Sprintf("%s", total),
		}
		valueJSON, err := json.Marshal(value)
		if err != nil {
			log.Println("Error marshalling timingMiddleware value:", err)
			return
		}
		_, _ = w.Write([]byte(fmt.Sprintf("<!--timingMiddleware:%s-->", valueJSON)))
	})
}

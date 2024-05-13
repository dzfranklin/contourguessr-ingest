package admin

import (
	"net/http"
)

func indexHandler(w http.ResponseWriter, r *http.Request) {
	templateResponse(w, r, "index.tmpl.html", nil)
}

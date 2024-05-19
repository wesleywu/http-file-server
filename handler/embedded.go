package handler

import (
	_ "embed"
	"net/http"
	"strings"
)

var (
	//go:embed static/layout/autoindex.css
	autoindex_css []byte
	//go:embed static/icons/blank.png
	blank_png []byte
	//go:embed static/icons/folder.png
	folder_png []byte
	//go:embed static/icons/go-previous.png
	go_previous_png []byte
	//go:embed static/icons/package-x-generic.png
	package_x_generic_png []byte
)

type EmbeddedHandler struct {
}

func (f *EmbeddedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	urlPath := r.URL.Path
	if !strings.HasPrefix(urlPath, "/static") {
		return
	}
	switch urlPath {
	case "/static/layout/autoindex.css":
		w.WriteHeader(200)
		w.Header().Set("Content-Type", "text/css")
		w.Write(autoindex_css)
	case "/static/icons/blank.png":
		w.WriteHeader(200)
		w.Header().Set("Content-Type", "image/png")
		w.Write(blank_png)
	case "/static/icons/folder.png":
		w.WriteHeader(200)
		w.Header().Set("Content-Type", "image/png")
		w.Write(folder_png)
	case "/static/icons/go-previous.png":
		w.WriteHeader(200)
		w.Header().Set("Content-Type", "image/png")
		w.Write(go_previous_png)
	case "/static/icons/package-x-generic.png":
		w.WriteHeader(200)
		w.Header().Set("Content-Type", "image/png")
		w.Write(package_x_generic_png)
	}
}

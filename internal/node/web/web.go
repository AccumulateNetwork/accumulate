package web

import (
	"embed"
	"net/http"
)

//go:embed *.svg *.html
var staticEmbeddedFS embed.FS

func staticEmbedded() http.FileSystem {
	return http.FS(staticEmbeddedFS)
}

func Handler() http.Handler {
	return http.FileServer(staticFS())
}

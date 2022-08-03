package web

import (
	"embed"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

//go:embed *.svg *.html
var staticEmbeddedFS embed.FS

func staticEmbedded() http.FileSystem {
	return http.FS(staticEmbeddedFS)
}

func staticFS() http.FileSystem {
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		return staticEmbedded()
	}

	_, err := os.Stat(file)
	if os.IsNotExist(err) {
		return staticEmbedded()
	}

	return http.Dir(filepath.Dir(file))
}

func Handler() http.Handler {
	return http.FileServer(staticFS())
}

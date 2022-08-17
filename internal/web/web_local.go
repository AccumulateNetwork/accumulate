//go:build web_local
// +build web_local

package web

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

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

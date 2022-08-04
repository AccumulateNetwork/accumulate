//go:build !web_local
// +build !web_local

package web

import "net/http"

func staticFS() http.FileSystem {
	return staticEmbedded()
}

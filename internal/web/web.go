// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

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

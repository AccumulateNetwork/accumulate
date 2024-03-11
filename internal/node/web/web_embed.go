// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !web_local
// +build !web_local

package web

import "net/http"

func staticFS() http.FileSystem {
	return staticEmbedded()
}

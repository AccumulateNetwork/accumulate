// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	_ "embed"
)

// the c template will include source + header with a ACME_HEADER guard between source and header.
//
//go:embed c.tmpl
var cSource string
var _ = Templates.Register(cSource, "c", nil, "c-source")

//go:embed c-header.tmpl
var cHeader string
var _ = Templates.Register(cHeader, "c-header", nil, "")

// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	_ "embed"
)

//go:embed kotlin.tmpl
var kotlinSrc string

var _ = Templates.Register(kotlinSrc, "kotlin", nil, "Kotlin")

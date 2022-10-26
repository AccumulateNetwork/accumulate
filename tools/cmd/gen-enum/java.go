// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	_ "embed"
)

//go:embed java.tmpl
var javaSrc string

var _ = Templates.Register(javaSrc, "java", nil, "Java")

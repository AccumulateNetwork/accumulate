// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	_ "embed"
)

//go:embed csharp.tmpl
var csSrc string

var _ = Templates.Register(csSrc, "csharp", nil, "CSharp")

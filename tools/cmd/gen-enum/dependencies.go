// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build tools
// +build tools

package main

// Importing this ensures that it will show up as a transitive dependency of
// modules that depend on this command
import (
	_ "github.com/rinchsan/gosimports/cmd/gosimports"
)

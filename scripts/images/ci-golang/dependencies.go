// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build tools
// +build tools

package accumulate

import (
	_ "github.com/sourcegraph/scip-go/cmd/scip-go"
	_ "github.com/sourcegraph/scip/cmd/scip"
)

// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build tools
// +build tools

package accumulate

import (
	_ "github.com/FactomProject/factom"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/rinchsan/gosimports/cmd/gosimports"
	_ "github.com/vektra/mockery/v2"
	_ "golang.org/x/tools/cmd/goimports"
	_ "gotest.tools/gotestsum"
)

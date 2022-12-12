// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build tools
// +build tools

package accumulate

// Force `go mod tidy` to include tool dependencies
import (
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/rinchsan/gosimports/cmd/gosimports"
	_ "github.com/vektra/mockery/v2"
	_ "gitlab.com/accumulatenetwork/core/wallet/cmd/accumulate"
	_ "golang.org/x/tools/cmd/goimports"
	_ "gotest.tools/gotestsum"
)

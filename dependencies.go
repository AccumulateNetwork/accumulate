//go:build tools
// +build tools

package wallet

// Force `go mod tidy` to include tool dependencies
import (
	_ "github.com/rinchsan/gosimports/cmd/gosimports"
	_ "gotest.tools/gotestsum"
)

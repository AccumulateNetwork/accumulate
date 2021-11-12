//go:build tools
// +build tools

package accumulate

// Force `go mod tidy` to include tool dependencies
import (
	_ "github.com/golang/mock/mockgen"
	_ "golang.org/x/tools/cmd/goimports"
)

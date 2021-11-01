//go:build tools
// +build tools

package accumulated

// Force `go mod tidy` to include tool dependencies
import (
	_ "github.com/golang/mock/mockgen"
)

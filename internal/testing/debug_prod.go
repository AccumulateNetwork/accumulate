//go:build production
// +build production

package testing

import (
	"fmt"
	"os"

	"github.com/fatih/color"
)

func EnableDebugFeatures() {
	fmt.Fprintln(os.Stderr, color.RedString("Debugging features are not supported in production"))
	// os.Exit(1)
}

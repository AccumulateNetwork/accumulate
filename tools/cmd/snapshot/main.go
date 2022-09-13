package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/testing"
)

var cmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Snapshot utilities",
}

func main() {
	// Allow snapshots to contain large values
	encoding.MaxValueSize = 1 << 30

	// Print detailed debug messages
	testing.EnableDebugFeatures()

	_ = cmd.Execute()
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fatalf("%+v", err)
	}
}

func checkf(err error, format string, otherArgs ...interface{}) {
	if err != nil {
		fatalf(format+": %+v", append(otherArgs, err)...)
	}
}

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

//go:generate go run ../gen-types --package main ../../../internal/database/types.yml -i accountState,merkleState,transactionState,sigSetData,SigSetEntry

var cmd = &cobra.Command{
	Use:   "debug",
	Short: "Accumulate daemon debug utilities",
}

func main() {
	_ = accountCmd.Execute()
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func checkf(err error, format string, otherArgs ...interface{}) {
	if err != nil {
		fatalf(format+": %v", append(otherArgs, err)...)
	}
}

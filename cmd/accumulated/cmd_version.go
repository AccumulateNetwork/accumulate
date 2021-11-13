package main

import (
	"fmt"
	"os"

	"github.com/AccumulateNetwork/accumulate"
	"github.com/spf13/cobra"
)

var cmdVersion = &cobra.Command{
	Use: "version",
	Run: showVersion,
}

var flagVersion struct {
	VersionOnly  bool
	KnownVersion bool
}

func init() {
	cmdMain.AddCommand(cmdVersion)

	cmdVersion.Flags().BoolVar(&flagVersion.VersionOnly, "version-only", false, "Only print out the version number")
	cmdVersion.Flags().BoolVar(&flagVersion.KnownVersion, "known-version", false, "Return 1 if the version number is unknown")
}

func showVersion(*cobra.Command, []string) {
	if flagVersion.KnownVersion && !accumulate.IsVersionKnown() {
		defer os.Exit(1)
	} else {
		defer os.Exit(0)
	}

	if flagVersion.VersionOnly {
		fmt.Println(accumulate.Version)
		return
	}

	fmt.Printf("%s %s\n", cmdMain.Short, accumulate.Version)
	fmt.Println(accumulate.Commit)
}

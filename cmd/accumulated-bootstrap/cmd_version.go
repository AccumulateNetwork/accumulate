// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate"
)

// `accumulated-bootstrap version` mirrors `accumulated version` so
// version-probing tools (notably accman's fleet poller) work uniformly
// across the binaries it manages. Without this, "<binary> version"
// returned "unknown command" and the deployed-version column for
// bootstrap nodes stayed blank in the fleet UI.
var cmdVersion = &cobra.Command{
	Use:   "version",
	Short: "Print version and exit",
	Run:   showVersion,
}

var flagVersion struct {
	VersionOnly  bool
	KnownVersion bool
}

func init() {
	cmd.AddCommand(cmdVersion)
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

	// Two-line format matches accumulated's: "<Short> <version>" then commit.
	fmt.Printf("%s %s\n", cmd.Short, accumulate.Version)
	fmt.Println(accumulate.Commit)
}

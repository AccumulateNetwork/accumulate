// Copyright 2024 The Accumulate Authors
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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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

	var name = "MainNet"
	if protocol.IsTestNet {
		name = "TestNet"
	}
	fmt.Printf("%s %s %s\n", cmdMain.Short, name, accumulate.Version)
	fmt.Println(accumulate.Commit)
}

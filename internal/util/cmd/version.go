// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package cmdutil

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// VersionCmd returns a cobra command that prints version information.
func VersionCmd() *cobra.Command {
	var versionOnly, knownVersion bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(*cobra.Command, []string) {
			if knownVersion && !accumulate.IsVersionKnown() {
				defer os.Exit(1)
			} else {
				defer os.Exit(0)
			}

			if versionOnly {
				fmt.Println(accumulate.Version)
				return
			}

			var name = "MainNet"
			if protocol.IsTestNet {
				name = "TestNet"
			}
			fmt.Printf("%s %s\n", name, accumulate.Version)
			fmt.Println(accumulate.Commit)
		},
	}

	cmd.Flags().BoolVar(&versionOnly, "version-only", false, "Only print out the version number")
	cmd.Flags().BoolVar(&knownVersion, "known-version", false, "Return 1 if the version number is unknown")

	return cmd
}

// AddVersionFlag adds a --version flag to a cobra command that prints version
// information and exits. This should be called in init() after creating the
// root command.
func AddVersionFlag(cmd *cobra.Command) {
	// Check os.Args early before cobra parses, since cobra validates required
	// args before running PersistentPreRun
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-version" {
			PrintVersion()
		}
		// Stop at first non-flag argument
		if len(arg) == 0 || arg[0] != '-' {
			break
		}
		// Stop at -- separator
		if arg == "--" {
			break
		}
	}

	// Also add the flag so it appears in help
	cmd.PersistentFlags().Bool("version", false, "Print version information and exit")
}

// PrintVersion prints version information and exits.
// Use this for non-cobra binaries with standard flag package.
func PrintVersion() {
	var name = "MainNet"
	if protocol.IsTestNet {
		name = "TestNet"
	}
	fmt.Printf("%s %s\n", name, accumulate.Version)
	fmt.Println(accumulate.Commit)
	os.Exit(0)
}

// CheckVersion checks os.Args for --version or version and prints version info
// if found. Use this at the start of main() for simple binaries that don't use
// cobra or the flag package.
func CheckVersion() {
	if len(os.Args) >= 2 && (os.Args[1] == "--version" || os.Args[1] == "version" || os.Args[1] == "-version") {
		PrintVersion()
	}
}

// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package cmdutil

import (
	"fmt"
	"io"
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

			fmt.Print(FormatVersion())
		},
	}

	cmd.Flags().BoolVar(&versionOnly, "version-only", false, "Only print out the version number")
	cmd.Flags().BoolVar(&knownVersion, "known-version", false, "Return 1 if the version number is unknown")

	return cmd
}

// FormatVersion returns the formatted version string with an optional prefix.
// If prefix is provided, output is "{prefix} {network} {version}".
// Otherwise, output is "{network} {version}".
func FormatVersion(prefix ...string) string {
	var network = "MainNet"
	if protocol.IsTestNet {
		network = "TestNet"
	}
	if len(prefix) > 0 && prefix[0] != "" {
		return fmt.Sprintf("%s %s %s\n%s\n", prefix[0], network, accumulate.Version, accumulate.Commit)
	}
	return fmt.Sprintf("%s %s\n%s\n", network, accumulate.Version, accumulate.Commit)
}

// WriteVersion writes version information to the given writer.
func WriteVersion(w io.Writer) {
	fmt.Fprint(w, FormatVersion())
}

// AddVersionFlag adds a --version flag to a cobra command that prints version
// information and exits. This should be called in init() after creating the
// root command.
func AddVersionFlag(cmd *cobra.Command) {
	// Check os.Args early before cobra parses, since cobra validates required
	// args before running PersistentPreRun
	if hasVersionFlag(os.Args[1:]) {
		PrintVersion()
	}

	// Also add the flag so it appears in help
	cmd.PersistentFlags().Bool("version", false, "Print version information and exit")
}

// hasVersionFlag checks if the given args contain --version or -version flag
// before any non-flag arguments or -- separator.
func hasVersionFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--version" || arg == "-version" {
			return true
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
	return false
}

// PrintVersion prints version information and exits.
// Use this for non-cobra binaries with standard flag package.
func PrintVersion() {
	fmt.Print(FormatVersion())
	os.Exit(0)
}

// CheckVersion checks os.Args for --version or version and prints version info
// if found. Use this at the start of main() for simple binaries that don't use
// cobra or the flag package.
func CheckVersion() {
	if isVersionArg(os.Args) {
		PrintVersion()
	}
}

// isVersionArg checks if the first argument is a version request.
func isVersionArg(args []string) bool {
	if len(args) < 2 {
		return false
	}
	return args[1] == "--version" || args[1] == "version" || args[1] == "-version"
}

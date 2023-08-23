// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

var cmd = &cobra.Command{
	Use:   "dbRepair",
	Short: "utilities to use a good db to build a repair script for a bad db",
}

var cmdBuildTestDBs = &cobra.Command{
	Use:   "buildTestDBs [number of entries] [good database] [bad database]",
	Short: "Number of entries must be greater than 100, should be greater than 5000",
	Args:  cobra.ExactArgs(3),
	Run:   runBuildTestDBs,
}

var cmdBuildSummary = &cobra.Command{
	Use:   "buildSummary [good database] [summary file]",
	Short: "Build a Summary file of the keys and values in a good database",
	Args:  cobra.ExactArgs(2),
	Run:   runBuildSummary,
}

var cmdBuildDiff = &cobra.Command{
	Use:   "buildDiff [summary file] [bad database] [diff File]",
	Short: "Given the summary data and the bad database, build a diff File to make the bad database good",
	Args:  cobra.ExactArgs(3),
	Run:   runBuildDiff,
}

var cmdPrintDiff = &cobra.Command{
	Use:   "printDiff [diff file] [good database] [diff File]",
	Short: "Given the summary data and the good database, print the diff file",
	Args:  cobra.ExactArgs(2),
	Run:   runPrintDiff,
}

var cmdBuildFix = &cobra.Command{
	Use:   "buildFix [diff file] [good database] [fix file]",
	Short: "Take the difference between the good and bad database, the good data base, and build a fix file",
	Args:  cobra.ExactArgs(3),
	Run:   runBuildFix,
}

var cmdApplyFix = &cobra.Command{
	Use:   "applyFix [fix file] [database]",
	Short: "Applies the fix to the database",
	Args:  cobra.ExactArgs(2),
	Run:   runApplyFix,
}

func init() {
	cmd.AddCommand(
		cmdBuildTestDBs,
		cmdBuildSummary,
		cmdBuildDiff,
		cmdPrintDiff,
		cmdBuildFix,
		cmdApplyFix,
	)
}

func main() {
	_ = cmd.Execute()
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		err = errors.UnknownError.Skip(1).Wrap(err)
		fatalf("%+v", err)
	}
}

func checkf(err error, format string, otherArgs ...interface{}) {
	if err != nil {
		fatalf(format+": %v", append(otherArgs, err)...)
	}
}

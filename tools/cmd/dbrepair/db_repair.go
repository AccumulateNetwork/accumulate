package main

import (
	"github.com/spf13/cobra"
)

var cmdBuildTestDBs = &cobra.Command{
	Use:   "buildTestDBs [number of entries] [good database] [bad database]",
	Short: "Number of entries must be greater than 100, should be greater than 5000",
	Args:  cobra.ExactArgs(3),
	Run:   runBuildTestDBs,
}

var cmdBuildSummary = &cobra.Command{
	Use:   "buildSummary [badger database] [summary file]",
	Short: "Build a Summary file using a good database",
	Args:  cobra.ExactArgs(2),
	Run:   runBuildSummary,
}

var cmdBuildDiff = &cobra.Command{
	Use:   "buildDiff [summary.dat] [bad database] [diff File]",
	Short: "Given the summary data and the bad database, build a diff File to make the bad database good",
	Args:  cobra.ExactArgs(3),
	Run:   runBuildDiff,
}

var cmdBuildFix = &cobra.Command{
	Use:   "buildFix [diff file] [good database] [fix File]",
	Short: "Take the difference between the good and bad database, the good data base, and build a fix file",
	Args:  cobra.ExactArgs(3),
	Run:   runBuildFix,
}

func init() {
	cmd.AddCommand(cmdBuildTestDBs)
	cmd.AddCommand(cmdBuildSummary)
	cmd.AddCommand(cmdBuildDiff)
	cmd.AddCommand(cmdBuildFix)
}

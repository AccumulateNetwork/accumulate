package main

import "github.com/spf13/cobra"

var cmdBvc = &cobra.Command{
	Use:   "bvc",
	Short: "Block validator chain",
	Run:   printUsageAndExit1,
}

func init() {
	cmdMain.AddCommand(cmdBvc)
}

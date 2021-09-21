package main

import "github.com/spf13/cobra"

var cmdDc = &cobra.Command{
	Use:   "dc",
	Short: "Directory chain",
	Run:   printUsageAndExit1,
}

func init() {
	cmdMain.AddCommand(cmdDc)
}
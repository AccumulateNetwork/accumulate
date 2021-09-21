package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cmdDcInit = &cobra.Command{
	Use:   "init",
	Short: "Initialize DC node",
	Run:   initDC,
}

func init() {
	cmdDc.AddCommand(cmdDcInit)
}

func initDC(*cobra.Command, []string) {
	fmt.Fprintf(os.Stderr, "This command has not yet been implemented\n")
	os.Exit(1)
}

package main

import (
	"fmt"
	"os"

	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/AccumulateNetwork/accumulated/networks"
	"github.com/spf13/cobra"
)

const defaultInitNet = "Acadia"

var cmdInit = &cobra.Command{
	Use:   "init",
	Short: "Initialize node",
	Run:   initNode,
}

var flagInit struct {
	Net string
}

func init() {
	cmdMain.AddCommand(cmdInit)

	cmdInit.Flags().StringVarP(&flagInit.Net, "network", "n", defaultInitNet, "Node to build configs for")
}

func initNode(*cobra.Command, []string) {
	i := networks.IndexOf(flagInit.Net)
	if i < 0 {
		fmt.Fprintf(os.Stderr, "Unknown network %q\n", flagInit.Net)
		os.Exit(1)
	}

	fmt.Printf("Building configs for %s\n", flagInit.Net)
	node.InitForNetwork("accumulate.", i, flagMain.WorkDir)
}

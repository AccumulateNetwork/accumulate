package main

import (
	"fmt"
	"os"

	"github.com/AccumulateNetwork/accumulated/blockchain/tendermint"
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
	for j := range networks.Networks {
		if networks.Networks[j].Name == flagInit.Net {
			fmt.Printf("Building configs for %s\n", flagInit.Net)
			tendermint.Initialize("accumulate.", j, flagMain.WorkDir)
			return
		}
	}

	fmt.Fprintf(os.Stderr, "Unknown network %q\n", flagInit.Net)
}

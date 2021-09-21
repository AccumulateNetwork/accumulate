package main

import (
	"fmt"
	"os"

	"github.com/AccumulateNetwork/accumulated/blockchain/tendermint"
	"github.com/AccumulateNetwork/accumulated/networks"
	"github.com/spf13/cobra"
)

const defaultBvcInitNet = "Acadia"

var cmdBvcInit = &cobra.Command{
	Use:   "init",
	Short: "Initialize BVC node",
	Run:   initBVC,
}

var flagBvcInit struct {
	Net string
}

func init() {
	cmdBvc.AddCommand(cmdBvcInit)

	cmdBvcInit.Flags().StringVarP(&flagBvcInit.Net, "network", "n", defaultBvcInitNet, "Node to build configs for")
}

func initBVC(*cobra.Command, []string) {
	for j := range networks.Networks {
		if networks.Networks[j].Name == flagBvcInit.Net {
			fmt.Printf("Building configs for %s\n", flagBvcInit.Net)
			tendermint.Initialize("accumulate.", j, flagMain.WorkDir)
			return
		}
	}

	fmt.Fprintf(os.Stderr, "Unknown network %q\n", flagBvcInit.Net)
}

package main

import (
	"fmt"
	"os"

	"github.com/AccumulateNetwork/accumulated/blockchain/tendermint"
	cfg "github.com/AccumulateNetwork/accumulated/config"
	"github.com/spf13/cobra"
	tmcfg "github.com/tendermint/tendermint/config"
)

var cmdTestNet = &cobra.Command{
	Use:   "testnet",
	Short: "Create a LAN testnet using 127.0.1.X",
	Run:   initTestNet,
}

func init() {
	cmdMain.AddCommand(cmdTestNet)
}

func initTestNet(cmd *cobra.Command, args []string) {
	const nValidators = 2
	const nNonValidators = 1

	if !cmd.Flag("work-dir").Changed {
		fmt.Fprint(os.Stderr, "Error: --work-dir is required\n")
		_ = cmd.Usage()
		os.Exit(1)
	}

	IPs := make([]string, nValidators+nNonValidators)
	for i := range IPs {
		IPs[i] = fmt.Sprintf("tcp://127.0.1.%d", i+1)
	}

	config := make([]*cfg.Config, nValidators+nNonValidators)
	for i := range config {
		config[i] = new(cfg.Config)
		if i < nValidators {
			config[i].Config = *tmcfg.DefaultValidatorConfig()
		} else {
			config[i].Config = *tmcfg.DefaultConfig()
		}
	}

	err := tendermint.InitWithConfig(flagMain.WorkDir, "TestNetLAN", "TestNetLAN", 26656, config, IPs, IPs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

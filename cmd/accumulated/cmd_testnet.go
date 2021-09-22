package main

import (
	"fmt"
	"net"
	"os"

	cfg "github.com/AccumulateNetwork/accumulated/config"
	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/spf13/cobra"
)

var cmdTestNet = &cobra.Command{
	Use:   "testnet",
	Short: "Create a LAN testnet using 127.0.1.X",
	Run:   initTestNet,
}

var flagTestNet struct {
	NumValidators int
	BasePort      int
	BaseIP        string
	NoEmptyBlocks bool
}

func init() {
	cmdMain.AddCommand(cmdTestNet)

	cmdTestNet.Flags().IntVarP(&flagTestNet.NumValidators, "validators", "v", 3, "Number of validator nodes to configure")
	cmdTestNet.Flags().IntVar(&flagTestNet.BasePort, "port", 26656, "Base port to use for listeners")
	cmdTestNet.Flags().StringVar(&flagTestNet.BaseIP, "ip", "127.0.1.1", "Base IP address for nodes - must not end with .0")
	cmdTestNet.Flags().BoolVar(&flagTestNet.NoEmptyBlocks, "no-empty-blocks", false, "Do not create empty blocks")
}

func initTestNet(cmd *cobra.Command, args []string) {
	baseIP := net.ParseIP(flagTestNet.BaseIP)
	if baseIP == nil {
		fmt.Fprintf(os.Stderr, "Error: %q is not a valid IP address\n", flagTestNet.BaseIP)
		cmd.Usage()
		os.Exit(1)
	}
	if baseIP[15] == 0 {
		fmt.Fprintf(os.Stderr, "Error: base IP address must not end with .0\n")
		cmd.Usage()
		os.Exit(1)
	}

	if !cmd.Flag("work-dir").Changed {
		fmt.Fprint(os.Stderr, "Error: --work-dir is required\n")
		_ = cmd.Usage()
		os.Exit(1)
	}

	n := flagTestNet.NumValidators
	IPs := make([]string, n)
	config := make([]*cfg.Config, n)

	for i := range IPs {
		ip := make(net.IP, len(baseIP))
		copy(ip, baseIP)
		ip[15] += byte(i)
		IPs[i] = fmt.Sprintf("tcp://%v", ip)
	}

	for i := range config {
		config[i] = cfg.DefaultValidator()
		if flagTestNet.NoEmptyBlocks {
			config[i].Consensus.CreateEmptyBlocks = false
		}
	}

	err := node.InitWithConfig(flagMain.WorkDir, "LocalhostTestNet", "LocalhostTestNet", flagTestNet.BasePort, config, IPs, IPs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

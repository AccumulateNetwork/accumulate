package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/AccumulateNetwork/accumulated/config"
	cfg "github.com/AccumulateNetwork/accumulated/config"
	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/AccumulateNetwork/accumulated/networks"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	tmcfg "github.com/tendermint/tendermint/config"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/types"
	"golang.org/x/term"
)

const defaultInitNet = "Acadia"

var cmdInit = &cobra.Command{
	Use:   "init [follower]",
	Short: "Initialize node",
	Run:   initNode,
}

var cmdInitFollower = &cobra.Command{
	Use:   "follower",
	Short: "Initialize follower node",
	Run:   initFollower,
}

var flagInit struct {
	Net string
}

var flagInitFollower struct {
	GenesisDoc string
	ListenIP   string
}

func init() {
	cmdMain.AddCommand(cmdInit)
	cmdInit.AddCommand(cmdInitFollower)

	cmdInit.PersistentFlags().StringVarP(&flagInit.Net, "network", "n", defaultInitNet, "Node to build configs for")
	cmdInit.MarkFlagRequired("network")

	cmdInitFollower.Flags().StringVar(&flagInitFollower.GenesisDoc, "genesis-doc", "", "Genesis doc for the target network")
	cmdInitFollower.Flags().StringVar(&flagInitFollower.ListenIP, "listen", "", "Address and port to listen on, e.g. tcp://1.2.3.4:5678")
	cmdInitFollower.MarkFlagRequired("listen")
}

func initNode(*cobra.Command, []string) {
	i := networks.IndexOf(flagInit.Net)
	if i < 0 {
		fmt.Fprintf(os.Stderr, "Unknown network %q\n", flagInit.Net)
		os.Exit(1)
	}
	network := networks.Networks[i]

	fmt.Printf("Building configs for %s\n", network.Name)

	listenIP := make([]string, len(network.Ip))
	config := make([]*cfg.Config, len(network.Ip))

	for i := range network.Ip {
		listenIP[i] = "tcp://0.0.0.0"
		config[i] = new(cfg.Config)
		config[i].Config = *tmcfg.DefaultValidatorConfig()
	}

	err := node.Init(node.InitOptions{
		WorkDir:   flagMain.WorkDir,
		ShardName: "accumulate.",
		ChainID:   network.Name,
		Port:      network.Port,
		Config:    config,
		RemoteIP:  network.Ip,
		ListenIP:  listenIP,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func initFollower(cmd *cobra.Command, args []string) {
	if !strings.HasPrefix(flagInitFollower.ListenIP, "tcp://") {
		fmt.Fprintf(os.Stderr, "Error: --listen must start with 'tcp://'\n")
		cmd.Usage()
		os.Exit(1)
	}

	i := networks.IndexOf(flagInit.Net)
	if i < 0 {
		fmt.Fprintf(os.Stderr, "Error: unknown network %q\n", flagInit.Net)
		os.Exit(1)
	}
	network := networks.Networks[i]

	var genDoc *types.GenesisDoc
	var err error
	if cmd.Flag("genesis-doc").Changed {
		genDoc, err = types.GenesisDocFromFile(flagInitFollower.GenesisDoc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load genesis doc %q: %v\n", flagInitFollower.GenesisDoc, err)
			os.Exit(1)
		}
	}

	peers := make([]string, len(network.Ip))
	for i, ip := range network.Ip {
		client, err := rpchttp.New(fmt.Sprintf("tcp://%s:%d", ip, network.Port+1))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to connect to %s: %v\n", ip, err)
			os.Exit(1)
		}

		if genDoc == nil {
			msg := "WARNING!!! You are fetching the Genesis document from %s! Only do this if you trust %[1]s and your connection to it!\n"
			if term.IsTerminal(int(os.Stderr.Fd())) {
				fmt.Fprint(os.Stderr, color.RedString(msg, ip))
			} else {
				fmt.Fprintf(os.Stderr, msg, ip)
			}
			rgen, err := client.Genesis(context.Background())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: failed to get genesis of %s: %v\n", ip, err)
				os.Exit(1)
			}
			genDoc = rgen.Genesis
		}

		status, err := client.Status(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to get status of %s: %v\n", ip, err)
			os.Exit(1)
		}

		peers[i] = fmt.Sprintf("%s@%s:%d", status.NodeInfo.NodeID, ip, network.Port)
	}

	config := config.Default()
	config.P2P.PersistentPeers = strings.Join(peers, ",")

	err = node.Init(node.InitOptions{
		WorkDir:    flagMain.WorkDir,
		ShardName:  "accumulate.",
		ChainID:    network.Name,
		Port:       network.Port,
		GenesisDoc: genDoc,
		Config:     []*cfg.Config{config},
		RemoteIP:   []string{""},
		ListenIP:   []string{flagInitFollower.ListenIP},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

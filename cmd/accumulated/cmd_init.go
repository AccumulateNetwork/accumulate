package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/AccumulateNetwork/accumulated/config"
	cfg "github.com/AccumulateNetwork/accumulated/config"
	"github.com/AccumulateNetwork/accumulated/internal/node"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	"github.com/AccumulateNetwork/accumulated/networks"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	tmcfg "github.com/tendermint/tendermint/config"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/types"
	"golang.org/x/term"
)

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
	Net   string
	Relay []string
}

var flagInitFollower struct {
	GenesisDoc string
	ListenIP   string
}

func init() {
	cmdMain.AddCommand(cmdInit)
	cmdInit.AddCommand(cmdInitFollower)

	cmdInit.PersistentFlags().StringVarP(&flagInit.Net, "network", "n", "", "Node to build configs for")
	cmdInit.PersistentFlags().StringSliceVarP(&flagInit.Relay, "relay-to", "r", nil, "Other networks that should be relayed to")
	cmdInit.MarkFlagRequired("network")

	cmdInitFollower.Flags().StringVar(&flagInitFollower.GenesisDoc, "genesis-doc", "", "Genesis doc for the target network")
	cmdInitFollower.Flags().StringVar(&flagInitFollower.ListenIP, "listen", "", "Address and port to listen on, e.g. tcp://1.2.3.4:5678")
	cmdInitFollower.MarkFlagRequired("listen")
}

func initNode(cmd *cobra.Command, args []string) {
	network := networks.Networks[flagInit.Net]
	if network == nil {
		fmt.Fprintf(os.Stderr, "Error: unknown network %q\n", flagInit.Net)
		os.Exit(1)
	}

	if !stringSliceContains(flagInit.Relay, flagInit.Net) {
		fmt.Fprintf(os.Stderr, "Error: the node's own network, %q, must be included in --relay-to\n", flagInit.Net)
		cmd.Usage()
		os.Exit(1)
	}

	_, err := relay.NewWith(flagInit.Relay...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: --relay-to: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Building config for %s\n", network.Name)

	listenIP := make([]string, len(network.Nodes))
	remoteIP := make([]string, len(network.Nodes))
	config := make([]*cfg.Config, len(network.Nodes))

	for i, net := range network.Nodes {
		listenIP[i] = "tcp://0.0.0.0"
		remoteIP[i] = net.IP
		config[i] = new(cfg.Config)
		config[i].Accumulate.Type = network.Type
		config[i].Accumulate.Networks = flagInit.Relay

		switch net.Type {
		case cfg.Validator:
			config[i].Config = *tmcfg.DefaultValidatorConfig()
		case cfg.Follower:
			config[i].Config = *tmcfg.DefaultValidatorConfig()
		default:
			fmt.Fprintf(os.Stderr, "Error: hard-coded network has invalid node type: %q\n", net.Type)
			os.Exit(1)
		}
	}

	err = node.Init(node.InitOptions{
		WorkDir:   flagMain.WorkDir,
		ShardName: "accumulate.",
		ChainID:   network.Name,
		Port:      network.Port,
		Config:    config,
		RemoteIP:  remoteIP,
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

	network := networks.Networks[flagInit.Net]
	if network == nil {
		fmt.Fprintf(os.Stderr, "Error: unknown network %q\n", flagInit.Net)
		os.Exit(1)
	}

	var genDoc *types.GenesisDoc
	var err error
	if cmd.Flag("genesis-doc").Changed {
		genDoc, err = types.GenesisDocFromFile(flagInitFollower.GenesisDoc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load genesis doc %q: %v\n", flagInitFollower.GenesisDoc, err)
			os.Exit(1)
		}
	}

	peers := make([]string, len(network.Nodes))
	for i, ip := range network.Nodes {
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

func stringSliceContains(s []string, t string) bool {
	for _, s := range s {
		if s == t {
			return true
		}
	}
	return false
}

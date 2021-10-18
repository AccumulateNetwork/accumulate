package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
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
	Net           string
	Relay         []string
	NoEmptyBlocks bool
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
	cmdInit.PersistentFlags().BoolVar(&flagInit.NoEmptyBlocks, "no-empty-blocks", false, "Do not create empty blocks")
	cmdInit.MarkFlagRequired("network")

	cmdInitFollower.Flags().StringVar(&flagInitFollower.GenesisDoc, "genesis-doc", "", "Genesis doc for the target network")
	cmdInitFollower.Flags().StringVarP(&flagInitFollower.ListenIP, "listen", "l", "", "Address and port to listen on, e.g. tcp://1.2.3.4:5678")
	cmdInitFollower.MarkFlagRequired("listen")
}

func initNode(cmd *cobra.Command, args []string) {
	network := networks.Networks[flagInit.Net]
	if network == nil {
		fatalf("unknown network %q", flagInit.Net)
	}

	if !stringSliceContains(flagInit.Relay, flagInit.Net) {
		fmt.Fprintf(os.Stderr, "Error: the node's own network, %q, must be included in --relay-to\n", flagInit.Net)
		printUsageAndExit1(cmd, args)
	}

	_, err := relay.NewWith(flagInit.Relay...)
	checkf(err, "--relay-to", err)

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
			fatalf("hard-coded network has invalid node type: %q", net.Type)
		}
		if flagInit.NoEmptyBlocks {
			config[i].Consensus.CreateEmptyBlocks = false
		}
	}

	check(node.Init(node.InitOptions{
		WorkDir:   flagMain.WorkDir,
		ShardName: "accumulate.",
		ChainID:   network.Name,
		Port:      network.Port,
		Config:    config,
		RemoteIP:  remoteIP,
		ListenIP:  listenIP,
	}))
}

func initFollower(cmd *cobra.Command, args []string) {
	u, err := url.Parse(flagInitFollower.ListenIP)
	checkf(err, "invalid --listen %q", flagInitFollower.ListenIP)

	port := 26656
	if u.Port() != "" {
		p, err := strconv.ParseInt(u.Port(), 10, 16)
		if err != nil {
			fatalf("invalid port number %q", u.Port())
		}
		port = int(p)
		u.Host = u.Host[:len(u.Host)-len(u.Port())-1]
	}

	network := networks.Networks[flagInit.Net]
	if network == nil {
		fatalf("unknown network %q", flagInit.Net)
	}

	var genDoc *types.GenesisDoc
	if cmd.Flag("genesis-doc").Changed {
		genDoc, err = types.GenesisDocFromFile(flagInitFollower.GenesisDoc)
		checkf(err, "failed to load genesis doc %q", flagInitFollower.GenesisDoc)
	}

	peers := make([]string, len(network.Nodes))
	for i, n := range network.Nodes {
		client, err := rpchttp.New(fmt.Sprintf("tcp://%s:%d", n.IP, network.Port+node.TmRpcPortOffset))
		checkf(err, "failed to connect to %s", n.IP)

		if genDoc == nil {
			msg := "WARNING!!! You are fetching the Genesis document from %s! Only do this if you trust %[1]s and your connection to it!\n"
			if term.IsTerminal(int(os.Stderr.Fd())) {
				fmt.Fprint(os.Stderr, color.RedString(msg, n.IP))
			} else {
				fmt.Fprintf(os.Stderr, msg, n.IP)
			}
			rgen, err := client.Genesis(context.Background())
			checkf(err, "failed to get genesis of %s", n.IP)
			genDoc = rgen.Genesis
		}

		status, err := client.Status(context.Background())
		checkf(err, "failed to get status of %s", n)

		peers[i] = fmt.Sprintf("%s@%s:%d", status.NodeInfo.NodeID, n.IP, network.Port)
	}

	config := config.Default()
	config.P2P.PersistentPeers = strings.Join(peers, ",")

	check(node.Init(node.InitOptions{
		WorkDir:    flagMain.WorkDir,
		ShardName:  "accumulate.",
		ChainID:    network.Name,
		Port:       port,
		GenesisDoc: genDoc,
		Config:     []*cfg.Config{config},
		RemoteIP:   []string{""},
		ListenIP:   []string{u.String()},
	}))
}

func stringSliceContains(s []string, t string) bool {
	for _, s := range s {
		if s == t {
			return true
		}
	}
	return false
}

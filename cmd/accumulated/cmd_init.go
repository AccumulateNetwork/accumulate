package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/AccumulateNetwork/accumulate/config"
	cfg "github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/node"
	"github.com/AccumulateNetwork/accumulate/internal/relay"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/types"
	"golang.org/x/term"
)

var cmdInit = &cobra.Command{
	Use:   "init",
	Short: "Initialize node",
	Run:   initNode,
	Args:  cobra.NoArgs,
}

var cmdInitFollower = &cobra.Command{
	Use:   "follower",
	Short: "Initialize follower node",
	Run:   initFollower,
	Args:  cobra.NoArgs,
}

var cmdInitDevnet = &cobra.Command{
	Use:   "devnet",
	Short: "Initialize a DevNet",
	Run:   initDevNet,
	Args:  cobra.NoArgs,
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

var flagInitDevnet struct {
	Name          string
	NumValidators int
	NumFollowers  int
	BasePort      int
	BaseIP        string
}

func init() {
	cmdMain.AddCommand(cmdInit)
	cmdInit.AddCommand(cmdInitFollower, cmdInitDevnet)

	cmdInit.PersistentFlags().StringVarP(&flagInit.Net, "network", "n", "", "Node to build configs for")
	cmdInit.PersistentFlags().StringSliceVarP(&flagInit.Relay, "relay-to", "r", nil, "Other networks that should be relayed to")
	cmdInit.PersistentFlags().BoolVar(&flagInit.NoEmptyBlocks, "no-empty-blocks", false, "Do not create empty blocks")
	cmdInit.MarkFlagRequired("network")
	cmdInit.MarkFlagRequired("relay-to")

	cmdInitFollower.Flags().StringVar(&flagInitFollower.GenesisDoc, "genesis-doc", "", "Genesis doc for the target network")
	cmdInitFollower.Flags().StringVarP(&flagInitFollower.ListenIP, "listen", "l", "", "Address and port to listen on, e.g. tcp://1.2.3.4:5678")
	cmdInitFollower.MarkFlagRequired("network")
	cmdInitFollower.MarkFlagRequired("listen")

	cmdInitDevnet.Flags().StringVar(&flagInitDevnet.Name, "name", "DevNet", "Network name")
	cmdInitDevnet.Flags().IntVarP(&flagInitDevnet.NumValidators, "validators", "v", 2, "Number of validator nodes to configure")
	cmdInitDevnet.Flags().IntVarP(&flagInitDevnet.NumFollowers, "followers", "f", 1, "Number of follower nodes to configure")
	cmdInitDevnet.Flags().IntVar(&flagInitDevnet.BasePort, "port", 26656, "Base port to use for listeners")
	cmdInitDevnet.Flags().StringVar(&flagInitDevnet.BaseIP, "ip", "127.0.1.1", "Base IP address for nodes - must not end with .0")
}

func initNode(cmd *cobra.Command, args []string) {
	network := networks.All[flagInit.Net]
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

		switch net.Type {
		case cfg.Validator:
			config[i] = cfg.DefaultValidator()
		case cfg.Follower:
			config[i] = cfg.Default()
		default:
			fatalf("hard-coded network has invalid node type: %q", net.Type)
		}

		if flagInit.NoEmptyBlocks {
			config[i].Consensus.CreateEmptyBlocks = false
		}

		config[i].Accumulate.Type = network.Type
		config[i].Accumulate.Networks = flagInit.Relay
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

func initFollower(cmd *cobra.Command, _ []string) {
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

	network := networks.All[flagInit.Net]
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

func initDevNet(cmd *cobra.Command, args []string) {
	if cmd.Flag("network").Changed {
		fatalf("--network is not applicable to devnet")
	}
	if cmd.Flag("relay-to").Changed {
		fatalf("--relay-to is not applicable to devnet")
	}

	baseIP := net.ParseIP(flagInitDevnet.BaseIP)
	if baseIP == nil {
		fmt.Fprintf(os.Stderr, "Error: %q is not a valid IP address\n", flagInitDevnet.BaseIP)
		printUsageAndExit1(cmd, args)
	}
	if baseIP[15] == 0 {
		fmt.Fprintf(os.Stderr, "Error: base IP address must not end with .0\n")
		printUsageAndExit1(cmd, args)
	}

	count := flagInitDevnet.NumValidators + flagInitDevnet.NumFollowers
	IPs := make([]string, count)
	config := make([]*cfg.Config, count)
	for i := range IPs {
		ip := make(net.IP, len(baseIP))
		copy(ip, baseIP)
		ip[15] += byte(i)
		IPs[i] = fmt.Sprintf("tcp://%v", ip)
	}

	for i := range config {
		if i < flagInitDevnet.NumValidators {
			config[i] = cfg.DefaultValidator()
		} else {
			config[i] = cfg.Default()
		}
		config[i].Accumulate.Networks = []string{fmt.Sprintf("%s:%d", IPs[0], flagInitDevnet.BasePort+node.TmRpcPortOffset)}
		if flagInit.NoEmptyBlocks {
			config[i].Consensus.CreateEmptyBlocks = false
		}
	}

	check(node.Init(node.InitOptions{
		WorkDir:   flagMain.WorkDir,
		ShardName: flagInitDevnet.Name,
		ChainID:   flagInitDevnet.Name,
		Port:      flagInitDevnet.BasePort,
		Config:    config,
		RemoteIP:  IPs,
		ListenIP:  IPs,
	}))
}

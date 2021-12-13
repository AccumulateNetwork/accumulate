package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/AccumulateNetwork/accumulate/config"
	cfg "github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/node"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/AccumulateNetwork/accumulate/protocol"
	dc "github.com/docker/cli/cli/compose/types"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/types"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
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
	NoEmptyBlocks bool
	NoWebsite     bool
	Reset         bool
}

var flagInitFollower struct {
	GenesisDoc string
	ListenIP   string
}

var flagInitDevnet struct {
	Name          string
	NumBvns       int
	NumValidators int
	NumFollowers  int
	BasePort      int
	BaseIP        string
	Docker        bool
	ComposeOnly   bool
}

func init() {
	cmdMain.AddCommand(cmdInit)
	cmdInit.AddCommand(cmdInitFollower, cmdInitDevnet)

	cmdInit.PersistentFlags().StringVarP(&flagInit.Net, "network", "n", "", "Node to build configs for")
	cmdInit.PersistentFlags().BoolVar(&flagInit.NoEmptyBlocks, "no-empty-blocks", false, "Do not create empty blocks")
	cmdInit.PersistentFlags().BoolVar(&flagInit.NoWebsite, "no-website", false, "Disable website")
	cmdInit.PersistentFlags().BoolVar(&flagInit.Reset, "reset", false, "Delete any existing directories within the working directory")
	cmdInit.MarkFlagRequired("network")

	cmdInitFollower.Flags().StringVar(&flagInitFollower.GenesisDoc, "genesis-doc", "", "Genesis doc for the target network")
	cmdInitFollower.Flags().StringVarP(&flagInitFollower.ListenIP, "listen", "l", "", "Address and port to listen on, e.g. tcp://1.2.3.4:5678")
	cmdInitFollower.MarkFlagRequired("network")
	cmdInitFollower.MarkFlagRequired("listen")

	cmdInitDevnet.Flags().StringVar(&flagInitDevnet.Name, "name", "DevNet", "Network name")
	cmdInitDevnet.Flags().IntVarP(&flagInitDevnet.NumBvns, "bvns", "b", 2, "Number of block validator networks to configure")
	cmdInitDevnet.Flags().IntVarP(&flagInitDevnet.NumValidators, "validators", "v", 2, "Number of validator nodes per subnet to configure")
	cmdInitDevnet.Flags().IntVarP(&flagInitDevnet.NumFollowers, "followers", "f", 1, "Number of follower nodes per subnet to configure")
	cmdInitDevnet.Flags().IntVar(&flagInitDevnet.BasePort, "port", 26656, "Base port to use for listeners")
	cmdInitDevnet.Flags().StringVar(&flagInitDevnet.BaseIP, "ip", "127.0.1.1", "Base IP address for nodes - must not end with .0")
	cmdInitDevnet.Flags().BoolVar(&flagInitDevnet.Docker, "docker", false, "Configure a network that will be deployed with Docker Compose")
	cmdInitDevnet.Flags().BoolVar(&flagInitDevnet.ComposeOnly, "compose-only", false, "Only write the Docker Compose file, do not write the configuration files")
}

func initNode(*cobra.Command, []string) {
	subnet, err := networks.Resolve(flagInit.Net)
	checkf(err, "--network")

	bvnNames := make([]string, 0, len(subnet.Network))
	addresses := map[string][]string{}
	index := map[string]int{}
	for _, s := range subnet.Network {
		if s.Type == cfg.BlockValidator {
			bvnNames = append(bvnNames, s.Name)
			index[s.Name] = s.Index
		}

		for _, n := range s.Nodes {
			addresses[s.Name] = append(addresses[s.Name], fmt.Sprintf("http://%s:%d", n.IP, s.Port))
		}
	}
	sort.Slice(bvnNames, func(i, j int) bool {
		return index[bvnNames[i]] < index[bvnNames[j]]
	})

	fmt.Printf("Building config for %s (%s)\n", subnet.Name, subnet.NetworkName)

	listenIP := make([]string, len(subnet.Nodes))
	remoteIP := make([]string, len(subnet.Nodes))
	config := make([]*cfg.Config, len(subnet.Nodes))

	for i, node := range subnet.Nodes {
		listenIP[i] = "tcp://0.0.0.0"
		remoteIP[i] = node.IP
		config[i] = cfg.Default(subnet.Type, node.Type, subnet.Name)

		if flagInit.NoEmptyBlocks {
			config[i].Consensus.CreateEmptyBlocks = false
		}

		if flagInit.NoWebsite {
			config[i].Accumulate.Website.Enabled = false
		}

		config[i].Accumulate.Network.BvnNames = bvnNames
		config[i].Accumulate.Network.Addresses = addresses
	}

	if flagInit.Reset {
		nodeReset()
	}

	check(node.Init(node.InitOptions{
		WorkDir:  flagMain.WorkDir,
		Port:     subnet.Port,
		Config:   config,
		RemoteIP: remoteIP,
		ListenIP: listenIP,
	}))
}

func nodeReset() {
	ent, err := os.ReadDir(flagMain.WorkDir)
	check(err)

	for _, ent := range ent {
		if !ent.IsDir() {
			continue
		}

		dir := path.Join(flagMain.WorkDir, ent.Name())
		fmt.Fprintf(os.Stderr, "Deleting %s\n", dir)
		err = os.RemoveAll(dir)
		check(err)
	}
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

	subnet, err := networks.Resolve(flagInit.Net)
	checkf(err, "--network")

	var genDoc *types.GenesisDoc
	if cmd.Flag("genesis-doc").Changed {
		genDoc, err = types.GenesisDocFromFile(flagInitFollower.GenesisDoc)
		checkf(err, "failed to load genesis doc %q", flagInitFollower.GenesisDoc)
	}

	peers := make([]string, len(subnet.Nodes))
	for i, n := range subnet.Nodes {
		client, err := rpchttp.New(fmt.Sprintf("tcp://%s:%d", n.IP, subnet.Port+networks.TmRpcPortOffset))
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

		peers[i] = fmt.Sprintf("%s@%s:%d", status.NodeInfo.NodeID, n.IP, subnet.Port)
	}

	config := config.Default(subnet.Type, cfg.Follower, subnet.Name)
	config.P2P.PersistentPeers = strings.Join(peers, ",")

	if flagInit.Reset {
		nodeReset()
	}

	check(node.Init(node.InitOptions{
		WorkDir:    flagMain.WorkDir,
		Port:       port,
		GenesisDoc: genDoc,
		Config:     []*cfg.Config{config},
		RemoteIP:   []string{""},
		ListenIP:   []string{u.String()},
	}))
}

func nextIP(baseIP net.IP) net.IP {
	ip := make(net.IP, len(baseIP))
	copy(ip, baseIP)
	baseIP[15]++
	return ip
}

func initDevNet(cmd *cobra.Command, args []string) {
	if cmd.Flag("network").Changed {
		fatalf("--network is not applicable to devnet")
	}

	if flagInitDevnet.ComposeOnly {
		flagInitDevnet.Docker = true
	}

	if flagInitDevnet.Docker && cmd.Flag("ip").Changed {
		fatalf("--ip and --docker are mutually exclusive")
	}

	if flagInitDevnet.NumBvns == 0 {
		fatalf("Must have at least one block validator network")
	}

	if flagInitDevnet.NumValidators == 0 {
		fatalf("Must have at least one block validator node")
	}

	baseIP := net.ParseIP(flagInitDevnet.BaseIP)
	if !flagInitDevnet.Docker {
		if baseIP == nil {
			fmt.Fprintf(os.Stderr, "Error: %q is not a valid IP address\n", flagInitDevnet.BaseIP)
			printUsageAndExit1(cmd, args)
		}
		if baseIP[15] == 0 {
			fmt.Fprintf(os.Stderr, "Error: base IP address must not end with .0\n")
			printUsageAndExit1(cmd, args)
		}
	}

	count := flagInitDevnet.NumValidators + flagInitDevnet.NumFollowers
	services := make([]dc.ServiceConfig, 0, 1+count*(flagInitDevnet.NumBvns+1))

	addresses := make(map[string][]string, flagInitDevnet.NumBvns+1)
	dnConfig := make([]*cfg.Config, count)
	dnRemote := make([]string, count)
	dnListen := make([]string, count)
	for i := 0; i < count; i++ {
		nodeType := cfg.Validator
		if i > flagInitDevnet.NumValidators {
			nodeType = cfg.Follower
		}
		var svc dc.ServiceConfig
		dnConfig[i], dnRemote[i], dnListen[i], svc = initDevNetNode(baseIP, cfg.Directory, nodeType, 0, i)
		addresses[protocol.Directory] = append(addresses[protocol.Directory], fmt.Sprintf("http://%s:%d", dnRemote[i], flagInitDevnet.BasePort))
		services = append(services, svc)
	}

	bvnConfig := make([][]*cfg.Config, flagInitDevnet.NumBvns)
	bvnRemote := make([][]string, flagInitDevnet.NumBvns)
	bvnListen := make([][]string, flagInitDevnet.NumBvns)
	bvns := make([]string, flagInitDevnet.NumBvns)
	for bvn := range bvnConfig {
		bvns[bvn] = fmt.Sprintf("BVN%d", bvn)
		bvnConfig[bvn] = make([]*cfg.Config, count)
		bvnRemote[bvn] = make([]string, count)
		bvnListen[bvn] = make([]string, count)
		for i := 0; i < count; i++ {
			nodeType := cfg.Validator
			if i > flagInitDevnet.NumValidators {
				nodeType = cfg.Follower
			}
			var svc dc.ServiceConfig
			bvnConfig[bvn][i], bvnRemote[bvn][i], bvnListen[bvn][i], svc = initDevNetNode(baseIP, cfg.BlockValidator, nodeType, bvn, i)
			addresses[bvns[bvn]] = append(addresses[bvns[bvn]], fmt.Sprintf("http://%s:%d", bvnRemote[bvn][i], flagInitDevnet.BasePort))
			services = append(services, svc)
		}
	}

	for _, c := range dnConfig {
		c.Accumulate.Network.BvnNames = bvns
		c.Accumulate.Network.Addresses = addresses
	}
	for _, c := range bvnConfig {
		for _, c := range c {
			c.Accumulate.Network.BvnNames = bvns
			c.Accumulate.Network.Addresses = addresses
		}
	}

	if flagInit.Reset {
		nodeReset()
	}

	if !flagInitDevnet.ComposeOnly {
		check(node.Init(node.InitOptions{
			WorkDir:  filepath.Join(flagMain.WorkDir, "dn"),
			Port:     flagInitDevnet.BasePort,
			Config:   dnConfig,
			RemoteIP: dnRemote,
			ListenIP: dnListen,
		}))
		for bvn := range bvnConfig {
			bvnConfig, bvnRemote, bvnListen := bvnConfig[bvn], bvnRemote[bvn], bvnListen[bvn]
			check(node.Init(node.InitOptions{
				WorkDir:  filepath.Join(flagMain.WorkDir, fmt.Sprintf("bvn%d", bvn)),
				Port:     flagInitDevnet.BasePort,
				Config:   bvnConfig,
				RemoteIP: bvnRemote,
				ListenIP: bvnListen,
			}))
		}
	}

	if !flagInitDevnet.Docker {
		return
	}

	var svc dc.ServiceConfig
	api := fmt.Sprintf("http://%s:%d/v1", dnRemote[0], flagInitDevnet.BasePort+networks.AccRouterJsonPortOffset)
	svc.Name = "scripts"
	svc.Image = "registry.gitlab.com/accumulatenetwork/accumulate/cli:latest"
	svc.Environment = map[string]*string{"ACC_API": &api}
	svc.Volumes = []dc.ServiceVolumeConfig{{Type: "bind", Source: ".", Target: "/node"}}
	services = append(services, svc)

	compose := new(dc.Config)
	compose.Version = "3"
	compose.Services = services

	services[0].Ports = make([]dc.ServicePortConfig, networks.MaxPortOffset+1)
	for i := range services[0].Ports {
		port := uint32(flagInitDevnet.BasePort + i)
		services[0].Ports[i] = dc.ServicePortConfig{
			Mode: "host", Protocol: "tcp", Target: port, Published: port,
		}
	}

	f, err := os.Create(filepath.Join(flagMain.WorkDir, "docker-compose.yml"))
	check(err)
	defer f.Close()

	err = yaml.NewEncoder(f).Encode(compose)
	check(err)
}

func initDevNetNode(baseIP net.IP, netType cfg.NetworkType, nodeType cfg.NodeType, bvn, node int) (config *cfg.Config, remote, listen string, svc dc.ServiceConfig) {
	if netType == cfg.Directory {
		config = cfg.Default(netType, nodeType, protocol.Directory)
	} else {
		config = cfg.Default(netType, nodeType, fmt.Sprintf("BVN%d", bvn))
	}

	if flagInit.NoEmptyBlocks {
		config.Consensus.CreateEmptyBlocks = false
	}
	if flagInit.NoWebsite {
		config.Accumulate.Website.Enabled = false
	}

	if !flagInitDevnet.Docker {
		ip := nextIP(baseIP).String()
		return config, ip, ip, dc.ServiceConfig{}
	}

	svc.Image = "registry.gitlab.com/accumulatenetwork/accumulate/accumulated:latest"
	svc.Volumes = make([]dc.ServiceVolumeConfig, 1)

	if netType == cfg.Directory {
		svc.Name = fmt.Sprintf("dn-%d", node)
		svc.Volumes[0] = dc.ServiceVolumeConfig{Type: "bind", Source: fmt.Sprintf("./dn/Node%d", node), Target: "/node"}
	} else {
		svc.Name = fmt.Sprintf("bvn%d-%d", bvn, node)
		svc.Volumes[0] = dc.ServiceVolumeConfig{Type: "bind", Source: fmt.Sprintf("./bvn%d/Node%d", bvn, node), Target: "/node"}
	}

	return config, svc.Name, "0.0.0.0", svc
}

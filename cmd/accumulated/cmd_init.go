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

	"github.com/AccumulateNetwork/accumulate"
	"github.com/AccumulateNetwork/accumulate/config"
	cfg "github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/internal/client"
	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/internal/node"
	"github.com/AccumulateNetwork/accumulate/networks"
	"github.com/AccumulateNetwork/accumulate/protocol"
	dc "github.com/docker/cli/cli/compose/types"
	"github.com/fatih/color"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/tendermint/tendermint/libs/log"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/types"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

var cmdInit = &cobra.Command{
	Use:   "init",
	Short: "Initialize a named network",
	Run:   initNamedNetwork,
	Args:  cobra.NoArgs,
}

var cmdInitNode = &cobra.Command{
	Use:   "node <network-name|url>",
	Short: "Initialize a node",
	Run:   initNode,
	Args:  cobra.ExactArgs(1),
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

var flagInitNode struct {
	GenesisDoc string
	ListenIP   string
	Follower   bool
}

var flagInitDevnet struct {
	Name          string
	NumBvns       int
	NumValidators int
	NumFollowers  int
	BasePort      int
	IPs           []string
	Docker        bool
	DockerTag     string
	UseVolumes    bool
	Compose       bool
}

func init() {
	cmdMain.AddCommand(cmdInit)
	cmdInit.AddCommand(cmdInitNode, cmdInitDevnet)

	cmdInit.PersistentFlags().StringVarP(&flagInit.Net, "network", "n", "", "Node to build configs for")
	cmdInit.PersistentFlags().BoolVar(&flagInit.NoEmptyBlocks, "no-empty-blocks", false, "Do not create empty blocks")
	cmdInit.PersistentFlags().BoolVar(&flagInit.NoWebsite, "no-website", false, "Disable website")
	cmdInit.PersistentFlags().BoolVar(&flagInit.Reset, "reset", false, "Delete any existing directories within the working directory")
	cmdInit.MarkFlagRequired("network")

	cmdInitNode.Flags().BoolVarP(&flagInitNode.Follower, "follow", "f", false, "Do not participate in voting")
	cmdInitNode.Flags().StringVar(&flagInitNode.GenesisDoc, "genesis-doc", "", "Genesis doc for the target network")
	cmdInitNode.Flags().StringVarP(&flagInitNode.ListenIP, "listen", "l", "", "Address and port to listen on, e.g. tcp://1.2.3.4:5678")
	cmdInitNode.MarkFlagRequired("listen")

	cmdInitDevnet.Flags().StringVar(&flagInitDevnet.Name, "name", "DevNet", "Network name")
	cmdInitDevnet.Flags().IntVarP(&flagInitDevnet.NumBvns, "bvns", "b", 2, "Number of block validator networks to configure")
	cmdInitDevnet.Flags().IntVarP(&flagInitDevnet.NumValidators, "validators", "v", 2, "Number of validator nodes per subnet to configure")
	cmdInitDevnet.Flags().IntVarP(&flagInitDevnet.NumFollowers, "followers", "f", 1, "Number of follower nodes per subnet to configure")
	cmdInitDevnet.Flags().IntVar(&flagInitDevnet.BasePort, "port", 26656, "Base port to use for listeners")
	cmdInitDevnet.Flags().StringSliceVar(&flagInitDevnet.IPs, "ip", []string{"127.0.1.1"}, "IP addresses to use or base IP - must not end with .0")
	cmdInitDevnet.Flags().BoolVar(&flagInitDevnet.Docker, "docker", false, "Configure a network that will be deployed with Docker Compose")
	cmdInitDevnet.Flags().StringVar(&flagInitDevnet.DockerTag, "tag", "latest", "Tag to use on the docker images")
	cmdInitDevnet.Flags().BoolVar(&flagInitDevnet.UseVolumes, "use-volumes", false, "Use Docker volumes instead of a local directory")
	cmdInitDevnet.Flags().BoolVar(&flagInitDevnet.Compose, "compose", false, "Only write the Docker Compose file, do not write the configuration files")
}

func initNamedNetwork(*cobra.Command, []string) {
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
		listenIP[i] = "0.0.0.0"
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
		Logger:   newLogger(),
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

func initNode(cmd *cobra.Command, args []string) {
	netAddr, netPort, err := networks.ResolveAddr(args[0], true)
	checkf(err, "invalid network name or URL")

	u, err := url.Parse(flagInitNode.ListenIP)
	checkf(err, "invalid --listen %q", flagInitNode.ListenIP)

	nodePort := 26656
	if u.Port() != "" {
		p, err := strconv.ParseInt(u.Port(), 10, 16)
		if err != nil {
			fatalf("invalid port number %q", u.Port())
		}
		nodePort = int(p)
		u.Host = u.Host[:len(u.Host)-len(u.Port())-1]
	}

	accClient, err := client.New(fmt.Sprintf("http://%s:%d", netAddr, netPort+networks.AccRouterJsonPortOffset))
	checkf(err, "failed to create API client for %s", args[0])

	tmClient, err := rpchttp.New(fmt.Sprintf("tcp://%s:%d", netAddr, netPort+networks.TmRpcPortOffset))
	checkf(err, "failed to create Tendermint client for %s", args[0])

	version := getVersion(accClient)
	switch {
	case !accumulate.IsVersionKnown() && !version.VersionIsKnown:
		// Hope for the best

	case accumulate.Commit != version.Commit:
		fatalf("wrong version: network is %s, we are %s", version.Commit, accumulate.Commit)
	}

	description, err := accClient.Describe(context.Background())
	checkf(err, "failed to get description from %s", args[0])

	var genDoc *types.GenesisDoc
	if cmd.Flag("genesis-doc").Changed {
		genDoc, err = types.GenesisDocFromFile(flagInitNode.GenesisDoc)
		checkf(err, "failed to load genesis doc %q", flagInitNode.GenesisDoc)
	} else {
		msg := "WARNING!!! You are fetching the Genesis document from %s! Only do this if you trust %[1]s and your connection to it!\n"
		if term.IsTerminal(int(os.Stderr.Fd())) {
			fmt.Fprint(os.Stderr, color.RedString(msg, args[0]))
		} else {
			fmt.Fprintf(os.Stderr, msg, args[0])
		}
		rgen, err := tmClient.Genesis(context.Background())
		checkf(err, "failed to get genesis from %s", args[0])
		genDoc = rgen.Genesis
	}

	status, err := tmClient.Status(context.Background())
	checkf(err, "failed to get status of %s", args[0])

	nodeType := cfg.Validator
	if flagInitNode.Follower {
		nodeType = cfg.Follower
	}
	config := config.Default(description.Subnet.Type, nodeType, description.Subnet.Name)
	config.P2P.PersistentPeers = fmt.Sprintf("%s@%s:%d", status.NodeInfo.NodeID, netAddr, netPort+networks.TmP2pPortOffset)

	if flagInit.Reset {
		nodeReset()
	}

	check(node.Init(node.InitOptions{
		WorkDir:    flagMain.WorkDir,
		Port:       nodePort,
		GenesisDoc: genDoc,
		Config:     []*cfg.Config{config},
		RemoteIP:   []string{""},
		ListenIP:   []string{u.String()},
		Logger:     newLogger(),
	}))
}

var baseIP net.IP
var ipCount byte

func nextIP() string {
	if len(flagInitDevnet.IPs) > 1 {
		ipCount++
		if len(flagInitDevnet.IPs) < int(ipCount) {
			fatalf("not enough IPs")
		}
		return flagInitDevnet.IPs[ipCount-1]
	}

	if baseIP == nil {
		baseIP = net.ParseIP(flagInitDevnet.IPs[0])
		if baseIP == nil {
			fatalf("invalid IP: %q", flagInitDevnet.IPs[0])
		}
		if baseIP[15] == 0 {
			fatalf("invalid IP: base IP address must not end with .0")
		}
	}

	ip := make(net.IP, len(baseIP))
	copy(ip, baseIP)
	ip[15] += ipCount
	ipCount++
	return ip.String()
}

func initDevNet(cmd *cobra.Command, _ []string) {
	if cmd.Flag("network").Changed {
		fatalf("--network is not applicable to devnet")
	}

	if flagInitDevnet.Compose {
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

	count := flagInitDevnet.NumValidators + flagInitDevnet.NumFollowers
	compose := new(dc.Config)
	compose.Version = "3"
	compose.Services = make([]dc.ServiceConfig, 0, 1+count*(flagInitDevnet.NumBvns+1))
	compose.Volumes = make(map[string]dc.VolumeConfig, 1+count*(flagInitDevnet.NumBvns+1))

	switch len(flagInitDevnet.IPs) {
	case 1:
		// Generate a sequence from the base IP
	case count * (flagInitDevnet.NumBvns + 1):
		// One IP per node
	default:
		fatalf("not enough IPs - you must specify one base IP or one IP for each node")
	}

	addresses := make(map[string][]string, flagInitDevnet.NumBvns+1)
	dnConfig := make([]*cfg.Config, count)
	dnRemote := make([]string, count)
	dnListen := make([]string, count)
	for i := 0; i < count; i++ {
		nodeType := cfg.Validator
		if i > flagInitDevnet.NumValidators {
			nodeType = cfg.Follower
		}
		dnConfig[i], dnRemote[i], dnListen[i] = initDevNetNode(cfg.Directory, nodeType, 0, i, compose)
		addresses[protocol.Directory] = append(addresses[protocol.Directory], fmt.Sprintf("http://%s:%d", dnRemote[i], flagInitDevnet.BasePort))
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
			bvnConfig[bvn][i], bvnRemote[bvn][i], bvnListen[bvn][i] = initDevNetNode(cfg.BlockValidator, nodeType, bvn, i, compose)
			addresses[bvns[bvn]] = append(addresses[bvns[bvn]], fmt.Sprintf("http://%s:%d", bvnRemote[bvn][i], flagInitDevnet.BasePort))
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

	if !flagInitDevnet.Compose {
		logger := newLogger()
		check(node.Init(node.InitOptions{
			WorkDir:  filepath.Join(flagMain.WorkDir, "dn"),
			Port:     flagInitDevnet.BasePort,
			Config:   dnConfig,
			RemoteIP: dnRemote,
			ListenIP: dnListen,
			Logger:   logger.With("subnet", protocol.Directory),
		}))
		for bvn := range bvnConfig {
			bvnConfig, bvnRemote, bvnListen := bvnConfig[bvn], bvnRemote[bvn], bvnListen[bvn]
			check(node.Init(node.InitOptions{
				WorkDir:  filepath.Join(flagMain.WorkDir, fmt.Sprintf("bvn%d", bvn)),
				Port:     flagInitDevnet.BasePort,
				Config:   bvnConfig,
				RemoteIP: bvnRemote,
				ListenIP: bvnListen,
				Logger:   logger.With("subnet", fmt.Sprintf("BVN%d", bvn)),
			}))
		}
		return
	}

	var svc dc.ServiceConfig
	api := fmt.Sprintf("http://%s:%d/v2", dnRemote[0], flagInitDevnet.BasePort+networks.AccRouterJsonPortOffset)
	svc.Name = "tools"
	svc.ContainerName = "devnet-init"
	svc.Image = "registry.gitlab.com/accumulatenetwork/accumulate/cli:" + flagInitDevnet.DockerTag
	svc.Environment = map[string]*string{"ACC_API": &api}

	svc.Command = dc.ShellCommand{"accumulated", "init", "devnet", "-w", "/nodes", "--docker"}
	cmd.Flags().Visit(func(flag *pflag.Flag) {
		switch flag.Name {
		case "work-dir", "docker", "compose", "reset":
			return
		}

		s := fmt.Sprintf("--%s=%v", flag.Name, flag.Value)
		svc.Command = append(svc.Command, s)
	})

	if flagInitDevnet.UseVolumes {
		svc.Volumes = make([]dc.ServiceVolumeConfig, len(compose.Services))
		for i, node := range compose.Services {
			bits := strings.SplitN(node.Name, "-", 2)
			svc.Volumes[i] = dc.ServiceVolumeConfig{Type: "volume", Source: node.Name, Target: path.Join("/nodes", bits[0], "Node"+bits[1])}
		}
	} else {
		svc.Volumes = []dc.ServiceVolumeConfig{
			{Type: "bind", Source: ".", Target: "/nodes"},
		}
	}

	compose.Services = append(compose.Services, svc)

	dn0svc := compose.Services[0]
	dn0svc.Ports = make([]dc.ServicePortConfig, networks.MaxPortOffset+1)
	for i := range dn0svc.Ports {
		port := uint32(flagInitDevnet.BasePort + i)
		dn0svc.Ports[i] = dc.ServicePortConfig{
			Mode: "host", Protocol: "tcp", Target: port, Published: port,
		}
	}

	f, err := os.Create(filepath.Join(flagMain.WorkDir, "docker-compose.yml"))
	check(err)
	defer f.Close()

	err = yaml.NewEncoder(f).Encode(compose)
	check(err)
}

func initDevNetNode(netType cfg.NetworkType, nodeType cfg.NodeType, bvn, node int, compose *dc.Config) (config *cfg.Config, remote, listen string) {
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
		ip := nextIP()
		return config, ip, ip
	}

	var name, dir string
	if netType == cfg.Directory {
		name, dir = fmt.Sprintf("dn-%d", node), fmt.Sprintf("./dn/Node%d", node)
	} else {
		name, dir = fmt.Sprintf("bvn%d-%d", bvn, node), fmt.Sprintf("./bvn%d/Node%d", bvn, node)
	}

	var svc dc.ServiceConfig
	svc.Name = name
	svc.ContainerName = "devnet-" + name
	svc.Image = "registry.gitlab.com/accumulatenetwork/accumulate/accumulated:" + flagInitDevnet.DockerTag
	svc.DependsOn = []string{"tools"}

	if flagInitDevnet.UseVolumes {
		svc.Volumes = []dc.ServiceVolumeConfig{
			{Type: "volume", Source: name, Target: "/node"},
		}
		compose.Volumes[name] = dc.VolumeConfig{}
	} else {
		svc.Volumes = []dc.ServiceVolumeConfig{
			{Type: "bind", Source: dir, Target: "/node"},
		}
	}

	compose.Services = append(compose.Services, svc)
	return config, svc.Name, "0.0.0.0"
}

func newLogger() log.Logger {
	writer, err := logging.NewConsoleWriter("plain")
	check(err)
	level, writer, err := logging.ParseLogLevel(config.DefaultLogLevels, writer)
	check(err)
	logger, err := logging.NewTendermintLogger(zerolog.New(writer), level, false)
	check(err)
	return logger
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	dc "github.com/docker/cli/cli/compose/types"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/tendermint/tendermint/libs/log"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/config"
	cfg "gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node"
	"gitlab.com/accumulatenetwork/accumulate/networks"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	etcd "go.etcd.io/etcd/client/v3"
	"gopkg.in/yaml.v3"
)

var cmdInit = &cobra.Command{
	Use:   "init",
	Short: "Initialize a named network",
	Run:   initNamedNetwork,
	Args:  cobra.NoArgs,
}

var cmdListNamedNetworkConfig = &cobra.Command{
	Use:   "list <network-name>",
	Short: "Initialize a named network",
	Run:   listNamedNetworkConfig,
	Args:  cobra.ExactArgs(1),
}

var cmdInitNode = &cobra.Command{
	Use:   "node <network-name|url>",
	Short: "Initialize a node",
	Run:   initNode,
	Args:  cobra.ExactArgs(1),
}

var cmdInitValidator = &cobra.Command{
	Use:   "network <subnet name>",
	Short: "Initialize a network",
	Run:   initValidator,
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
	LogLevels     string
	Etcd          []string
}

var flagInitNode struct {
	GenesisDoc       string
	ListenIP         string
	Follower         bool
	SkipVersionCheck bool
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
	DnsSuffix     string
}

var flagInitNetwork struct {
	NetworksFileName string
	Subnet           string
	GenesisDoc       string
	Docker           bool
	DockerTag        string
	UseVolumes       bool
	Compose          bool
	DnsSuffix        string
}

func init() {
	cmdMain.AddCommand(cmdInit)
	cmdInit.AddCommand(cmdInitNode, cmdInitDevnet, cmdInitValidator, cmdListNamedNetworkConfig)

	cmdInitValidator.Flags().StringVar(&flagInitNetwork.NetworksFileName, "networks", "networks.json", "Networks file name")
	cmdInitValidator.MarkFlagRequired("networks")

	cmdInitValidator.Flags().StringVar(&flagInitNetwork.Subnet, "subnet", "BVN0", "Specify the subnet to configure within the network")
	//cmdInitValidator.MarkFlagRequired("subnet")
	cmdInitValidator.Flags().StringVar(&flagInitNetwork.GenesisDoc, "genesis-doc", "", "Genesis doc for the target network")
	cmdInitValidator.Flags().BoolVar(&flagInitNetwork.Docker, "docker", false, "Configure a network that will be deployed with Docker Compose")
	cmdInitValidator.Flags().StringVar(&flagInitNetwork.DockerTag, "tag", "latest", "Tag to use on the docker images")
	cmdInitValidator.Flags().BoolVar(&flagInitNetwork.UseVolumes, "use-volumes", false, "Use Docker volumes instead of a local directory")
	cmdInitValidator.Flags().BoolVar(&flagInitNetwork.Compose, "compose", false, "Only write the Docker Compose file, do not write the configuration files")
	cmdInitValidator.Flags().StringVar(&flagInitNetwork.DnsSuffix, "dns-suffix", "", "DNS suffix to add to hostnames used when initializing dockerized nodes")

	cmdInit.PersistentFlags().StringVarP(&flagInit.Net, "network", "n", "", "Node to build configs for")
	cmdInit.PersistentFlags().BoolVar(&flagInit.NoEmptyBlocks, "no-empty-blocks", false, "Do not create empty blocks")
	cmdInit.PersistentFlags().BoolVar(&flagInit.NoWebsite, "no-website", false, "Disable website")
	cmdInit.PersistentFlags().BoolVar(&flagInit.Reset, "reset", false, "Delete any existing directories within the working directory")
	cmdInit.PersistentFlags().StringVar(&flagInit.LogLevels, "log-levels", "", "Override the default log levels")
	cmdInit.PersistentFlags().StringSliceVar(&flagInit.Etcd, "etcd", nil, "Use etcd endpoint(s)")
	cmdInit.MarkFlagRequired("network")

	cmdInitNode.Flags().BoolVarP(&flagInitNode.Follower, "follow", "f", false, "Do not participate in voting")
	cmdInitNode.Flags().StringVar(&flagInitNode.GenesisDoc, "genesis-doc", "", "Genesis doc for the target network")
	cmdInitNode.Flags().StringVarP(&flagInitNode.ListenIP, "listen", "l", "", "Address and port to listen on, e.g. tcp://1.2.3.4:5678")
	cmdInitNode.Flags().BoolVar(&flagInitNode.SkipVersionCheck, "skip-version-check", false, "Do not enforce the version check")
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
	cmdInitDevnet.Flags().StringVar(&flagInitDevnet.DnsSuffix, "dns-suffix", "", "DNS suffix to add to hostnames used when initializing dockerized nodes")
}

func listNamedNetworkConfig(_ *cobra.Command, args []string) {
	n := Network{}
	n.Network = "TestNet"
	for _, subnet := range networks.TestNet {
		s := Subnet{}

		s.Name = subnet.Name
		for i := range subnet.Nodes {
			s.Nodes = append(s.Nodes, Node{subnet.Nodes[i].IP, subnet.Nodes[i].Type})
		}
		s.Port = subnet.Port
		s.Type = subnet.Type
		n.Subnet = append(n.Subnet, s)
	}
	data, err := json.Marshal(&n)
	check(err)
	fmt.Printf("%s", data)
}

func initNamedNetwork(*cobra.Command, []string) {

	lclSubnet, err := networks.Resolve(flagInit.Net)
	checkf(err, "--network")

	subnets := make([]config.Subnet, len(lclSubnet.Network))
	i := uint(0)
	for _, s := range lclSubnet.Network {
		bvnNodes := make([]config.Node, len(s.Nodes))
		for i, n := range s.Nodes {
			bvnNodes[i] = config.Node{
				Type:    n.Type,
				Address: fmt.Sprintf("http://%s:%d", n.IP, s.Port),
			}
		}

		subnets[i] = config.Subnet{
			ID:    s.Name,
			Type:  s.Type,
			Nodes: bvnNodes,
		}
		i++
	}

	fmt.Printf("Building config for %s (%s)\n", lclSubnet.Name, lclSubnet.NetworkName)

	listenIP := make([]string, len(lclSubnet.Nodes))
	remoteIP := make([]string, len(lclSubnet.Nodes))
	config := make([]*cfg.Config, len(lclSubnet.Nodes))

	for i, node := range lclSubnet.Nodes {
		listenIP[i] = "0.0.0.0"
		remoteIP[i] = node.IP
		config[i] = cfg.Default(lclSubnet.Type, node.Type, lclSubnet.Name)
		config[i].Accumulate.Network.LocalAddress = fmt.Sprintf("%s:%d", node.IP, lclSubnet.Port)
		config[i].Accumulate.Network.Subnets = subnets

		if flagInit.LogLevels != "" {
			_, _, err := logging.ParseLogLevel(flagInit.LogLevels, io.Discard)
			checkf(err, "--log-level")
			config[i].LogLevel = flagInit.LogLevels
		}

		if flagInit.NoEmptyBlocks {
			config[i].Consensus.CreateEmptyBlocks = false
		}

		if flagInit.NoWebsite {
			config[i].Accumulate.Website.Enabled = false
		}
	}

	if flagInit.Reset {
		nodeReset()
	}

	check(node.Init(node.InitOptions{
		WorkDir:  flagMain.WorkDir,
		Port:     lclSubnet.Port,
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
	}

	accClient, err := client.New(fmt.Sprintf("http://%s:%d", netAddr, netPort+networks.AccRouterJsonPortOffset))
	checkf(err, "failed to create API client for %s", args[0])

	tmClient, err := rpchttp.New(fmt.Sprintf("tcp://%s:%d", netAddr, netPort+networks.TmRpcPortOffset))
	checkf(err, "failed to create Tendermint client for %s", args[0])

	version := getVersion(accClient)
	switch {
	case !accumulate.IsVersionKnown() && !version.VersionIsKnown:
		warnf("The version of this executable and %s is unknown. If there is a version mismatch, the node may fail.", args[0])

	case accumulate.Commit != version.Commit:
		if flagInitNode.SkipVersionCheck {
			warnf("This executable is version %s but %s is %s. This may cause the node to fail.", formatVersion(accumulate.Version, accumulate.IsVersionKnown()), args[0], formatVersion(version.Version, version.VersionIsKnown))
		} else {
			fatalf("wrong version: network is %s, we are %s", formatVersion(version.Version, version.VersionIsKnown), formatVersion(accumulate.Version, accumulate.IsVersionKnown()))
		}
	}

	description, err := accClient.Describe(context.Background())
	checkf(err, "failed to get description from %s", args[0])

	var genDoc *types.GenesisDoc
	if cmd.Flag("genesis-doc").Changed {
		genDoc, err = types.GenesisDocFromFile(flagInitNode.GenesisDoc)
		checkf(err, "failed to load genesis doc %q", flagInitNode.GenesisDoc)
	} else {
		warnf("You are fetching the Genesis document from %s! Only do this if you trust %[1]s and your connection to it!", args[0])
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
	config := config.Default(description.Subnet.Type, nodeType, description.Subnet.LocalSubnetID)
	config.P2P.PersistentPeers = fmt.Sprintf("%s@%s:%d", status.NodeInfo.NodeID, netAddr, netPort+networks.TmP2pPortOffset)
	config.Accumulate.Network = description.Subnet
	if flagInit.LogLevels != "" {
		_, _, err := logging.ParseLogLevel(flagInit.LogLevels, io.Discard)
		checkf(err, "--log-level")
		config.LogLevel = flagInit.LogLevels
	}

	if len(flagInit.Etcd) > 0 {
		config.Accumulate.Storage.Type = cfg.EtcdStorage
		config.Accumulate.Storage.Etcd = new(etcd.Config)
		config.Accumulate.Storage.Etcd.Endpoints = flagInit.Etcd
		config.Accumulate.Storage.Etcd.DialTimeout = 5 * time.Second
	}

	if flagInit.Reset {
		nodeReset()
	}

	check(node.Init(node.InitOptions{
		WorkDir:    flagMain.WorkDir,
		Port:       nodePort,
		GenesisDoc: genDoc,
		Config:     []*cfg.Config{config},
		RemoteIP:   []string{u.Hostname()},
		ListenIP:   []string{u.Hostname()},
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
	count := flagInitDevnet.NumValidators + flagInitDevnet.NumFollowers
	verifyInitFlags(cmd, count)

	compose := new(dc.Config)
	compose.Version = "3"
	compose.Services = make([]dc.ServiceConfig, 0, 1+count*(flagInitDevnet.NumBvns+1))
	compose.Volumes = make(map[string]dc.VolumeConfig, 1+count*(flagInitDevnet.NumBvns+1))

	subnets := make([]config.Subnet, flagInitDevnet.NumBvns+1)
	dnConfig := make([]*cfg.Config, count)
	dnRemote := make([]string, count)
	dnListen := make([]string, count)
	initDNs(count, dnConfig, dnRemote, dnListen, compose, subnets)

	bvnConfig := make([][]*cfg.Config, flagInitDevnet.NumBvns)
	bvnRemote := make([][]string, flagInitDevnet.NumBvns)
	bvnListen := make([][]string, flagInitDevnet.NumBvns)
	initBVNs(bvnConfig, count, bvnRemote, bvnListen, compose, subnets)

	handleDNSSuffix(dnRemote, bvnRemote)

	if flagInit.Reset {
		nodeReset()
	}

	if !flagInitDevnet.Compose {
		createInLocalFS(dnConfig, dnRemote, dnListen, bvnConfig, bvnRemote, bvnListen)
		return
	}
	createDockerCompose(cmd, dnRemote, compose)
}

func verifyInitFlags(cmd *cobra.Command, count int) {
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

	switch len(flagInitDevnet.IPs) {
	case 1:
		// Generate a sequence from the base IP
	case count * (flagInitDevnet.NumBvns + 1):
		// One IP per node
	default:
		fatalf("not enough IPs - you must specify one base IP or one IP for each node")
	}
}

func initDNs(count int, dnConfig []*cfg.Config, dnRemote []string, dnListen []string, compose *dc.Config, subnets []cfg.Subnet) {
	dnNodes := make([]config.Node, count)
	for i := 0; i < count; i++ {
		nodeType := cfg.Validator
		if i >= flagInitDevnet.NumValidators {
			nodeType = cfg.Follower
		}
		dnConfig[i], dnRemote[i], dnListen[i] = initDevNetNode(cfg.Directory, nodeType, 0, i, compose)
		dnNodes[i] = config.Node{
			Type:    nodeType,
			Address: fmt.Sprintf(fmt.Sprintf("http://%s:%d", dnRemote[i], flagInitDevnet.BasePort)),
		}
		dnConfig[i].Accumulate.Network.Subnets = subnets
		dnConfig[i].Accumulate.Network.LocalAddress = parseHost(dnNodes[i].Address)
	}

	subnets[0] = config.Subnet{
		ID:    protocol.Directory,
		Type:  config.Directory,
		Nodes: dnNodes,
	}
}

func initBVNs(bvnConfigs [][]*cfg.Config, count int, bvnRemotes [][]string, bvnListen [][]string, compose *dc.Config, subnets []cfg.Subnet) {
	for bvn := range bvnConfigs {
		subnetID := fmt.Sprintf("BVN%d", bvn)
		bvnConfigs[bvn] = make([]*cfg.Config, count)
		bvnRemotes[bvn] = make([]string, count)
		bvnListen[bvn] = make([]string, count)
		bvnNodes := make([]config.Node, count)

		for i := 0; i < count; i++ {
			nodeType := cfg.Validator
			if i >= flagInitDevnet.NumValidators {
				nodeType = cfg.Follower
			}
			bvnConfigs[bvn][i], bvnRemotes[bvn][i], bvnListen[bvn][i] = initDevNetNode(cfg.BlockValidator, nodeType, bvn, i, compose)
			bvnNodes[i] = config.Node{
				Type:    nodeType,
				Address: fmt.Sprintf("http://%s:%d", bvnRemotes[bvn][i], flagInitDevnet.BasePort),
			}
			bvnConfigs[bvn][i].Accumulate.Network.Subnets = subnets
			bvnConfigs[bvn][i].Accumulate.Network.LocalAddress = parseHost(bvnNodes[i].Address)
		}
		subnets[bvn+1] = config.Subnet{
			ID:    subnetID,
			Type:  config.BlockValidator,
			Nodes: bvnNodes,
		}
	}
}

func handleDNSSuffix(dnRemote []string, bvnRemote [][]string) {
	if flagInitDevnet.Docker && flagInitDevnet.DnsSuffix != "" {
		for i := range dnRemote {
			dnRemote[i] += flagInitDevnet.DnsSuffix
		}
		for _, remotes := range bvnRemote {
			for i := range remotes {
				remotes[i] += flagInitDevnet.DnsSuffix
			}
		}
	}
}

func createInLocalFS(dnConfig []*cfg.Config, dnRemote []string, dnListen []string, bvnConfig [][]*cfg.Config, bvnRemote [][]string, bvnListen [][]string) {
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
}

func createDockerCompose(cmd *cobra.Command, dnRemote []string, compose *dc.Config) {
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
	if flagInit.LogLevels != "" {
		_, _, err := logging.ParseLogLevel(flagInit.LogLevels, io.Discard)
		checkf(err, "--log-level")
		config.LogLevel = flagInit.LogLevels
	}

	if flagInit.NoEmptyBlocks {
		config.Consensus.CreateEmptyBlocks = false
	}
	if flagInit.NoWebsite {
		config.Accumulate.Website.Enabled = false
	}

	if len(flagInit.Etcd) > 0 {
		config.Accumulate.Storage.Type = cfg.EtcdStorage
		config.Accumulate.Storage.Etcd = new(etcd.Config)
		config.Accumulate.Storage.Etcd.Endpoints = flagInit.Etcd
		config.Accumulate.Storage.Etcd.DialTimeout = 5 * time.Second
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

func parseHost(address string) string {
	nodeUrl, err := url.Parse(address)
	if err == nil {
		return nodeUrl.Host
	}
	return address
}

func newLogger() log.Logger {
	levels := config.DefaultLogLevels
	if flagInit.LogLevels != "" {
		levels = flagInit.LogLevels
	}

	writer, err := logging.NewConsoleWriter("plain")
	check(err)
	level, writer, err := logging.ParseLogLevel(levels, writer)
	check(err)
	logger, err := logging.NewTendermintLogger(zerolog.New(writer), level, false)
	check(err)
	return logger
}

package main

import (
	"encoding/json"
	"fmt"
	dc "github.com/docker/cli/cli/compose/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gitlab.com/accumulatenetwork/accumulate/config"
	cfg "gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node"
	"gitlab.com/accumulatenetwork/accumulate/networks"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Ports struct {
	api  int
	p2p  int
	rpc  int
	grpc int
	acc  int
}

var dnBasePort = 16691
var bvnBasePort = 16651

var dnPorts = Ports{16691, 16692, 16693, 16694, 16695}
var bvnPorts = Ports{16651, 16652, 16653, 16654, 16655}

type Node struct {
	IP   string          `json:"ip"`
	Type config.NodeType `json:"type"`
}
type Subnet struct {
	Name  string             `json:"name"`
	Type  config.NetworkType `json:"type"`
	Port  int                `json:"port"`
	Nodes []Node             `json:"nodes"`
}

type Network struct {
	Network string   `json:"network"`
	Subnet  []Subnet `json:"subnet"`
}

func loadNetworkConfiguration(file string) (ret Network, err error) {
	jsonFile, err := os.Open(file)
	defer jsonFile.Close()
	// if we os.Open returns an error then handle it
	if err != nil {
		return ret, err
	}
	data, _ := ioutil.ReadAll(jsonFile)
	err = json.Unmarshal(data, &ret)
	return ret, err
}

//load network config file
func initValidator(cmd *cobra.Command, args []string) {

	if cmd.Flag("network").Changed {
		fatalf("--network is not applicable to accumulated init validator")
	}

	network, err := loadNetworkConfiguration(flagInitNetwork.NetworksFileName)

	fmt.Printf("%v", network)
	check(err)

	subnetName := args[0]
	var subnet *Subnet
	var directory *Subnet

	var bvns []string
	//now look for the subnet.
	for i, v := range network.Subnet {
		if strings.EqualFold(v.Name, subnetName) {
			if v.Type == config.Directory {
				check(fmt.Errorf("subnet configuration type cannot be a directory"))
			}
			subnet = &network.Subnet[i]
		}
		//while we are at it, also find the directory.
		if v.Type == config.Directory {
			directory = &network.Subnet[i]
		} else {
			bvns = append(bvns, v.Name)
		}
	}

	if subnet == nil {
		check(fmt.Errorf("cannot find subnet (%v) in %v", subnetName, flagInitNetwork.NetworksFileName))
	}

	if directory == nil {
		check(fmt.Errorf("cannot find directory configuration in %v", flagInitNetwork.NetworksFileName))
	}

	numBvns := len(network.Subnet) - 1

	if flagInitNetwork.Compose {
		flagInitNetwork.Docker = true
	}

	//if flagInitNetwork.Docker && cmd.Flag("ip").Changed {
	//	fatalf("--ip and --docker are mutually exclusive")
	//}

	count := 1 //specify only 1 validator to start with flagInitDevnet.NumValidators
	compose := new(dc.Config)
	compose.Version = "3"
	compose.Services = make([]dc.ServiceConfig, 0, 1+count*(numBvns+1))
	compose.Volumes = make(map[string]dc.VolumeConfig, 1+count*(numBvns+1))

	switch len(flagInitDevnet.IPs) {
	case 1:
		// Generate a sequence from the base IP
	case count * (numBvns + 1):
		// One IP per node
	default:
		fatalf("not enough IPs - you must specify one base IP or one IP for each node")
	}

	addresses := make(map[string][]string, len(network.Subnet))
	dnConfig := make([]*cfg.Config, 1)
	var dnRemote []string
	dnListen := make([]string, count)

	//deed to configure the dn for each
	dnConfig[0], dnRemote, dnListen[0] = initNetworkNode(network.Network, directory.Name, directory.Nodes, subnet.Nodes, cfg.Directory,
		cfg.Validator, 0, 0, compose)

	bvnConfig := make([]*cfg.Config, 1) //numBvns)
	var bvnRemote []string

	bvnListen := make([]string, numBvns)
	bvnConfig[0], bvnRemote, bvnListen[0] = initNetworkNode(network.Network, subnet.Name, subnet.Nodes, subnet.Nodes, cfg.BlockValidator,
		cfg.Validator, 0, 0, compose)

	if flagInitNetwork.Docker && flagInitNetwork.DnsSuffix != "" {
		for i := range dnRemote {
			dnRemote[i] += flagInitNetwork.DnsSuffix
		}
		for i := range bvnRemote {
			bvnRemote[i] += flagInitNetwork.DnsSuffix
		}
	}

	for i := 0; i < len(directory.Nodes); i++ {
		addresses[protocol.Directory] = append(addresses[protocol.Directory], fmt.Sprintf("http://%s:%d", directory.Nodes[i].IP, directory.Port))
	}

	for _, sub := range network.Subnet {
		for _, bvn := range sub.Nodes {
			if sub.Type == config.BlockValidator {
				addresses[sub.Name] = append(addresses[sub.Name], fmt.Sprintf("http://%s:%d", bvn.IP, sub.Port))
			}
		}
	}

	for _, c := range dnConfig {
		c.Accumulate.Network.BvnNames = bvns
		c.Accumulate.Network.Addresses = addresses
	}
	for _, c := range bvnConfig {

		c.Accumulate.Network.BvnNames = bvns
		c.Accumulate.Network.Addresses = addresses
	}

	if flagInit.Reset {
		nodeReset()
	}
	//
	////if we neet to bootstrap we need to fetch genesis document and set persistent peers
	//accClient, err := client.New(fmt.Sprintf("http://%s:%d", subnet, subnet.Port+networks.AccRouterJsonPortOffset))
	//checkf(err, "failed to create API client for %s", args[0])
	//
	//tmClient, err := rpchttp.New(fmt.Sprintf("tcp://%s:%d", netAddr, subnet.Port+networks.TmRpcPortOffset))
	//checkf(err, "failed to create Tendermint client for %s", args[0])
	//
	//version := getVersion(accClient)
	//switch {
	//case !accumulate.IsVersionKnown() && !version.VersionIsKnown:
	//	warnf("The version of this executable and %s is unknown. If there is a version mismatch, the node may fail.", args[0])
	//
	//case accumulate.Commit != version.Commit:
	//	if flagInitNode.SkipVersionCheck {
	//		warnf("This executable is version %s but %s is %s. This may cause the node to fail.", formatVersion(accumulate.Version, accumulate.IsVersionKnown()), args[0], formatVersion(version.Version, version.VersionIsKnown))
	//	} else {
	//		fatalf("wrong version: network is %s, we are %s", formatVersion(version.Version, version.VersionIsKnown), formatVersion(accumulate.Version, accumulate.IsVersionKnown()))
	//	}
	//}
	//
	//description, err := accClient.Describe(context.Background())
	//checkf(err, "failed to get description from %s", args[0])
	//
	//var genDoc *types.GenesisDoc
	//if cmd.Flag("genesis-doc").Changed {
	//	genDoc, err = types.GenesisDocFromFile(flagInitNode.GenesisDoc)
	//	checkf(err, "failed to load genesis doc %q", flagInitNode.GenesisDoc)
	//} else {
	//	warnf("You are fetching the Genesis document from %s! Only do this if you trust %[1]s and your connection to it!", args[0])
	//	rgen, err := tmClient.Genesis(context.Background())
	//	checkf(err, "failed to get genesis from %s", args[0])
	//	genDoc = rgen.Genesis
	//}
	//
	//config := config.Default(description.Subnet.Type, config.Validator, description.Subnet.ID)
	//config.P2P.PersistentPeers = fmt.Sprintf("%s@%s:%d", status.NodeInfo.NodeID, netAddr, netPort+networks.TmP2pPortOffset)
	//config.Accumulate.Network = description.Subnet
	//if flagInit.LogLevels != "" {
	//	_, _, err := logging.ParseLogLevel(flagInit.LogLevels, io.Discard)
	//	checkf(err, "--log-level")
	//	config.LogLevel = flagInit.LogLevels
	//}

	if !flagInitNetwork.Compose {
		logger := newLogger()
		check(node.Init(node.InitOptions{
			WorkDir:  filepath.Join(flagMain.WorkDir, "dn"),
			Port:     directory.Port,
			Config:   dnConfig,
			RemoteIP: dnRemote,
			ListenIP: dnListen,
			Logger:   logger.With("subnet", protocol.Directory),
		}))

		check(node.Init(node.InitOptions{
			WorkDir:  filepath.Join(flagMain.WorkDir, strings.ToLower(subnet.Name)),
			Port:     subnet.Port,
			Config:   bvnConfig,
			RemoteIP: bvnRemote,
			ListenIP: bvnListen,
			Logger:   logger.With("subnet", subnet.Name),
		}))
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

	//err = yaml.NewEncoder(f).Encode(compose)
	check(err)

	//	initValidatorNode("dn", dnBasePort, cmd, args)
}

func initNetworkNode(networkName string, subnetName string, nodes []Node, local []Node, netType cfg.NetworkType, nodeType cfg.NodeType, bvn, node int, compose *dc.Config) (config *cfg.Config, remote []string, listen string) {
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

	var remotes []string
	if !flagInitNetwork.Docker {
		for _, v := range nodes {
			remotes = append(remotes, v.IP)
		}
		//need to find the bvn that mates the current
		return config, remotes, local[0].IP
	}

	var name, dir string
	if netType == cfg.Directory {
		name, dir = fmt.Sprintf("dn-%d", node), fmt.Sprintf("./dn/Node%d", node)
	} else {
		name, dir = fmt.Sprintf("bvn%d-%d", bvn, node), fmt.Sprintf("./%s/Node%d", strings.ToLower(subnetName), node)
	}

	var svc dc.ServiceConfig
	svc.Name = name
	svc.ContainerName = networkName + "-" + name
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
	remotes = append(remotes, svc.Name)
	return config, remotes, "0.0.0.0"
}

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

	var directory *Subnet
	var bvns []*Subnet

	//now look for the subnet.
	for i, v := range network.Subnet {
		//while we are at it, also find the directory.
		if v.Type == config.Directory {
			if directory != nil {
				check(fmt.Errorf("more than one directory subnet is defined, can only have 1"))
			}
			directory = &network.Subnet[i]
		}
	}

	if directory == nil {
		check(fmt.Errorf("cannot find directory configuration in %v", flagInitNetwork.NetworksFileName))
	}

	//var localAddress string

	//quick validation to make sure the directory node maps to each of the BVN's defined
	for _, dnn := range directory.Nodes {
		found := false
		for _, bvn := range network.Subnet {
			if bvn.Type == config.Directory {
				continue
			}
			for _, v := range bvn.Nodes {
				if strings.EqualFold(dnn.IP, v.IP) {
					bvns = append(bvns, &bvn)
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			check(fmt.Errorf("%s is defined in the directory nodes networks file, but has no supporting BVN node", dnn.IP))
		}
	}
	//if localAddress == "" {
	//	check(fmt.Errorf("cannot find local address for directory node"))
	//}

	// we'll need 1 DN for each BVN.
	numBvns := len(network.Subnet) - 1

	// if length of the directory !=

	if flagInitNetwork.Compose {
		flagInitNetwork.Docker = true
	}

	//if flagInitNetwork.Docker && cmd.Flag("ip").Changed {
	//	fatalf("--ip and --docker are mutually exclusive")
	//}

	count := 1 //we will only need a count of 1 since the bvn and dn will be run in the same app
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

	addresses := make(map[string][]string, len(directory.Nodes))
	dnConfig := make([]*cfg.Config, len(directory.Nodes))
	dnRemote := make([][]string, len(directory.Nodes))
	dnListen := make([]string, len(directory.Nodes))

	var accSub []config.Subnet
	for _, sub := range network.Subnet {
		s := config.Subnet{}
		s.ID = sub.Name
		s.Type = sub.Type
		for _, a := range sub.Nodes {
			address := fmt.Sprintf("http://%s:%d", a.IP, sub.Port)
			n := config.Node{}
			n.Address = address
			n.Type = a.Type
			s.Nodes = append(s.Nodes, n)
		}
		accSub = append(accSub, s)
	}

	//need to configure the dn for each BVN assuming 1 bvn
	for i, _ := range directory.Nodes {
		dnConfig[i], dnRemote[i], dnListen[i] = initNetworkNode(network.Network, directory.Name, directory.Nodes, bvns[i].Nodes, cfg.Directory,
			cfg.Validator, i, i, compose)

		if flagInitNetwork.Docker && flagInitNetwork.DnsSuffix != "" {
			for j := range dnRemote[i] {
				dnRemote[i][j] += flagInitNetwork.DnsSuffix
			}
		}
		c := dnConfig[i]
		if flagInit.NoEmptyBlocks {
			c.Consensus.CreateEmptyBlocks = false
		}

		if flagInit.NoWebsite {
			c.Accumulate.Website.Enabled = false
		}

		c.Accumulate.Network.LocalSubnetID = directory.Name
		c.Accumulate.Network.Type = directory.Type
		c.Accumulate.Network.Subnets = accSub
		c.Accumulate.Network.LocalAddress = fmt.Sprintf("%s:%d", bvns[i].Nodes[0].IP, directory.Port)
	}

	bvnConfig := make([][]*cfg.Config, len(bvns))
	bvnListen := make([][]string, len(bvns))
	for i, _ := range bvns {
		bvnConfig[i] = make([]*cfg.Config, 1)
		bvnListen[i] = make([]string, 1)
	}

	bvnRemote := make([][]string, numBvns)

	for i, v := range bvns {
		bvnConfig[i][0], bvnRemote[i], bvnListen[i][0] = initNetworkNode(network.Network, v.Name, v.Nodes, v.Nodes, cfg.BlockValidator,
			cfg.Validator, i, i, compose)
		if flagInitNetwork.Docker && flagInitNetwork.DnsSuffix != "" {
			for j := range bvnRemote[i] {
				bvnRemote[i][j] += flagInitNetwork.DnsSuffix
			}
		}
	}

	for _, sub := range network.Subnet {
		for _, bvn := range sub.Nodes {
			if sub.Type == config.BlockValidator {
				addresses[sub.Name] = append(addresses[sub.Name], fmt.Sprintf("http://%s:%d", bvn.IP, sub.Port))
			}
		}
	}

	for i, v := range bvns {
		for _, c := range bvnConfig[i] {
			if flagInit.NoEmptyBlocks {
				c.Consensus.CreateEmptyBlocks = false
			}

			if flagInit.NoWebsite {
				c.Accumulate.Website.Enabled = false
			}
			c.Accumulate.Network.LocalAddress = addresses[v.Name][0]
			c.Accumulate.Network.Type = config.BlockValidator
			c.Accumulate.Network.LocalSubnetID = v.Name
			c.Accumulate.Network.LocalAddress = fmt.Sprintf("%s:%d", v.Nodes[0].IP, v.Port)
			c.Accumulate.Network.Subnets = accSub
		}
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
			RemoteIP: dnRemote[0],
			ListenIP: dnListen,
			Logger:   logger.With("subnet", protocol.Directory),
		}))

		for i, _ := range directory.Nodes {
			check(node.Init(node.InitOptions{
				WorkDir:  filepath.Join(flagMain.WorkDir, fmt.Sprintf("bvn%d", i)),
				Port:     bvns[i].Port,
				Config:   bvnConfig[i],
				RemoteIP: bvnRemote[i],
				ListenIP: bvnListen[i],
				Logger:   logger.With("subnet", bvns[i].Name),
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

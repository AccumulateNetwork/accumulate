package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	dc "github.com/docker/cli/cli/compose/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gitlab.com/accumulatenetwork/accumulate/config"
	cfg "gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/genesis"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func loadNetworkConfiguration(file string) (ret config.Network, err error) {
	jsonFile, err := os.Open(file)
	defer func() { _ = jsonFile.Close() }()
	// if we os.Open returns an error then handle it
	if err != nil {
		return ret, err
	}
	data, _ := io.ReadAll(jsonFile)
	err = json.Unmarshal(data, &ret)
	return ret, err
}

//load network config file
func initNetwork(cmd *cobra.Command, args []string) {
	networkConfigFile := args[0]
	network, err := loadNetworkConfiguration(networkConfigFile)
	check(err)

	var directory *config.Subnet
	var bvns []*config.Subnet

	var bvnSubnet []*config.Subnet

	//now look for the subnet.
	for i, v := range network.Subnets {
		//while we are at it, also find the directory.
		if v.Type == config.Directory {
			if directory != nil {
				fatalf("more than one directory subnet is defined, can only have 1")
			}
			directory = &network.Subnets[i]
		}
		if v.Type == config.BlockValidator {
			bvnSubnet = append(bvnSubnet, &network.Subnets[i])
		}
	}

	if directory == nil {
		fatalf("cannot find directory configuration in %v", networkConfigFile)
		panic("not reached") // For static analysis
	}

	if directory.Id != "Directory" {
		fatalf("directory name specified in file was %s, but accumulated requires it to be \"Directory\"", directory.Id)
	}
	//quick validation to make sure the directory node maps to each of the BVN's defined
	for _, dnn := range directory.Nodes {
		found := false
		for _, bvn := range network.Subnets {
			if bvn.Type == config.Directory {
				continue
			}
			for _, v := range bvn.Nodes {
				if strings.EqualFold(dnn.Address, v.Address) {
					bvn := bvn // See docs/developer/rangevarref.md
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
			fatalf("%s is defined in the directory nodes networks file, but has no supporting BVN node", dnn.Address)
		}
	}

	// we'll need 1 DN for each BVN.
	numBvns := len(network.Subnets) - 1

	if flagInitNetwork.Compose {
		flagInitNetwork.Docker = true
	}

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

	dnConfig := make([]*cfg.Config, len(directory.Nodes))
	var dnRemote []string
	dnListen := make([]string, len(directory.Nodes))

	//need to configure the dn for each BVN assuming 1 bvn
	for i := range directory.Nodes {
		var remotes []string
		dnConfig[i], remotes, dnListen[i] = initNetworkNode(network.Id, directory.Id, directory.Nodes, cfg.Directory,
			cfg.Validator, i, i, compose)

		dnRemote = append(dnRemote, remotes...)
		if flagInitNetwork.Docker && flagInitNetwork.DnsSuffix != "" {
			for j := range dnRemote {
				dnRemote[j] += flagInitNetwork.DnsSuffix
			}
		}
		c := dnConfig[i]
		if flagInit.NoEmptyBlocks {
			c.Consensus.CreateEmptyBlocks = false
		}

		if flagInit.NoWebsite {
			c.Accumulate.Website.Enabled = false
		}
		dnListen[i] = "0.0.0.0"
		c.Accumulate.SubnetId = directory.Id
		c.Accumulate.NetworkType = directory.Type
		c.Accumulate.Network = network
		c.Accumulate.LocalAddress = fmt.Sprintf("%s:%d", bvns[i].Nodes[0].Address, directory.BasePort)
	}

	bvnConfig := make([][]*cfg.Config, len(bvnSubnet))
	bvnListen := make([][]string, len(bvnSubnet))
	for i, v := range bvnSubnet {
		bvnConfig[i] = make([]*cfg.Config, len(v.Nodes))
		bvnListen[i] = make([]string, len(v.Nodes))
	}

	bvnRemote := make([][]string, len(bvnSubnet))

	for i, v := range bvnSubnet {
		for j := range v.Nodes {
			bvnConfig[i][j], bvnRemote[i], bvnListen[i][j] = initNetworkNode(network.Id, v.Id, v.Nodes, cfg.BlockValidator,
				cfg.Validator, i, j, compose)
			if flagInitNetwork.Docker && flagInitNetwork.DnsSuffix != "" {
				for j := range bvnRemote[i] {
					bvnRemote[i][j] += flagInitNetwork.DnsSuffix
				}
			}
			c := bvnConfig[i][j]
			if flagInit.NoEmptyBlocks {
				c.Consensus.CreateEmptyBlocks = false
			}

			if flagInit.NoWebsite {
				c.Accumulate.Website.Enabled = false
			}
			c.Accumulate.NetworkType = config.BlockValidator
			c.Accumulate.SubnetId = v.Id
			c.Accumulate.LocalAddress = fmt.Sprintf("%s:%d", v.Nodes[j].Address, v.BasePort)
			c.Accumulate.Network = network // Subnets = accSub
		}
	}

	for i, v := range bvnSubnet {
		for j := range v.Nodes {
			c := bvnConfig[i][j]
			if flagInit.NoEmptyBlocks {
				c.Consensus.CreateEmptyBlocks = false
			}

			if flagInit.NoWebsite {
				c.Accumulate.Website.Enabled = false
			}
			c.Accumulate.NetworkType = config.BlockValidator
			c.Accumulate.SubnetId = v.Id
			c.Accumulate.LocalAddress = fmt.Sprintf("%s:%d", v.Nodes[j].Address, v.BasePort)
			c.Accumulate.Network = network
		}
	}

	if flagInit.Reset {
		nodeReset()
	}
	var factomAddressesFile string
	if flagInitNetwork.FactomBalances != "" {
		factomAddressesFile = flagInitNetwork.FactomBalances
	}

	if !flagInitNetwork.Compose {
		logger := newLogger()
		netValMap := make(genesis.NetworkOperators)
		genInit, err := node.Init(node.InitOptions{
			WorkDir:             filepath.Join(flagMain.WorkDir, "dn"),
			Port:                int(directory.BasePort),
			Config:              dnConfig,
			RemoteIP:            dnRemote,
			ListenIP:            dnListen,
			NetworkOperators:    netValMap,
			Logger:              logger.With("subnet", protocol.Directory),
			FactomAddressesFile: factomAddressesFile,
		})
		check(err)
		genList := []genesis.Bootstrap{genInit}

		for i := range bvnSubnet {
			genInit, err := node.Init(node.InitOptions{
				WorkDir:             filepath.Join(flagMain.WorkDir, fmt.Sprintf("bvn%d", i)),
				Port:                int(bvns[i].BasePort),
				Config:              bvnConfig[i],
				RemoteIP:            bvnRemote[i],
				ListenIP:            bvnListen[i],
				NetworkOperators:    netValMap,
				Logger:              logger.With("subnet", bvns[i].Id),
				FactomAddressesFile: factomAddressesFile,
			})
			check(err)
			if genInit != nil {
				genList = append(genList, genInit)
			}
		}

		// Execute bootstrap after the entire network is known
		for _, genesis := range genList {
			err := genesis.Bootstrap()
			if err != nil {
				panic(fmt.Errorf("could not execute genesis: %v", err))
			}
		}
		return
	}

	var svc dc.ServiceConfig
	api := fmt.Sprintf("http://%s:%d/v2", dnRemote[0], flagInitDevnet.BasePort+int(cfg.PortOffsetAccumulateApi))
	svc.Name = "tools"
	svc.ContainerName = "devnet-init"
	svc.Image = flagInitDevnet.DockerImage
	svc.Environment = map[string]*string{"ACC_API": &api}

	svc.Command = dc.ShellCommand{"init", "devnet", "-w", "/nodes", "--docker"}

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
	dn0svc.Ports = make([]dc.ServicePortConfig, int(config.PortOffsetMax)+1)
	for i := range dn0svc.Ports {
		port := uint32(flagInitDevnet.BasePort + i)
		dn0svc.Ports[i] = dc.ServicePortConfig{
			Mode: "host", Protocol: "tcp", Target: port, Published: port,
		}
	}

	f, err := os.Create(filepath.Join(flagMain.WorkDir, "docker-compose.yml"))
	check(err)
	defer f.Close()

	check(err)
}

func initNetworkNode(networkName string, subnetName string, nodes []config.Node, netType cfg.NetworkType, nodeType cfg.NodeType,
	bvn, node int, compose *dc.Config) (config *cfg.Config, remote []string, listen string) {

	if netType == cfg.Directory {
		config = cfg.Default(networkName, netType, nodeType, protocol.Directory)
	} else {
		config = cfg.Default(networkName, netType, nodeType, fmt.Sprintf("BVN%d", bvn))
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
			remotes = append(remotes, v.Address)
		}
		//need to find the bvn that mates the current
		return config, remotes, "0.0.0.0"
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
	svc.Image = flagInitDevnet.DockerImage
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

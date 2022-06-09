package main

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	dc "github.com/docker/cli/cli/compose/types"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/config"
	cfg "gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/genesis"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node"
	"gitlab.com/accumulatenetwork/accumulate/networks"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	etcd "go.etcd.io/etcd/client/v3"
)

func initDevNet(cmd *cobra.Command, _ []string) {
	count := flagInitDevnet.NumValidators + flagInitDevnet.NumFollowers
	verifyInitFlags(cmd, count)

	compose := new(dc.Config)
	compose.Version = "3"
	compose.Services = make([]dc.ServiceConfig, 0, 1+count*(flagInitDevnet.NumBvns+1))
	compose.Volumes = make(map[string]dc.VolumeConfig, 1+count*(flagInitDevnet.NumBvns+1))

	network := new(config.Network)
	var configs [][]*cfg.Config
	var nodeDirs, remotes, listens [][]string
	var valKeys [][]crypto.PrivKey

	dn := cfg.Subnet{
		ID:   protocol.Directory,
		Type: config.Directory,
	}
	network.Subnets = append(network.Subnets, &dn)
	configs = append(configs, []*cfg.Config{})
	nodeDirs = append(nodeDirs, []string{})
	listens = append(listens, []string{})
	remotes = append(remotes, []string{})
	valKeys = append(valKeys, []crypto.PrivKey{})

	var nodeNum int
	for i := 0; i < flagInitDevnet.NumBvns; i++ {
		bvn := &cfg.Subnet{
			ID:   fmt.Sprintf("BVN%d", i+1),
			Type: config.BlockValidator,
		}
		network.Subnets = append(network.Subnets, bvn)
		configs = append(configs, []*cfg.Config{})
		nodeDirs = append(nodeDirs, []string{})
		listens = append(listens, []string{})
		remotes = append(remotes, []string{})
		valKeys = append(valKeys, []crypto.PrivKey{})

		for j := 0; j < count; j++ {
			nodeNum++
			typ := cfg.Follower
			if j < flagInitDevnet.NumValidators {
				typ = cfg.Validator
			}

			valKey := ed25519.GenPrivKey()
			valKeys[0] = append(valKeys[0], valKey)
			valKeys[i+1] = append(valKeys[i+1], valKey)

			name := fmt.Sprintf("node-%d", nodeNum)
			nodeDirs[0] = append(nodeDirs[0], filepath.Join(name, "dnn"))
			nodeDirs[i+1] = append(nodeDirs[i+1], filepath.Join(name, "bvnn"))

			// Create BVNN
			config, remote, listen := initDevNetNode(cfg.BlockValidator, typ, name, bvn.ID, compose)
			configs[i+1] = append(configs[i+1], config)
			listens[i+1] = append(listens[i+1], remote)
			remotes[i+1] = append(remotes[i+1], listen)
			bvn.Nodes = append(bvn.Nodes, &cfg.Node{
				Type:    typ,
				Address: fmt.Sprintf("http://%s:%d", remote, flagInitDevnet.BasePort),
			})

			config.Accumulate.Network.LocalAddress = fmt.Sprintf("%s:%d", remote, flagInitDevnet.BasePort)

			// Create DNN
			config = cfg.Default(cfg.DevNet, cfg.Directory, typ, protocol.Directory)
			configDevNet(config)
			config.Instrumentation.Prometheus = false // Do not run prometheus on the DNN
			configs[0] = append(configs[0], config)
			listens[0] = append(listens[0], remote)
			remotes[0] = append(remotes[0], listen)
			dn.Nodes = append(dn.Nodes, &cfg.Node{
				Type:    typ,
				Address: fmt.Sprintf("http://%s:%d", remote, flagInitDevnet.BasePort+networks.DnnPortOffset),
			})

			config.Accumulate.Network.LocalAddress = fmt.Sprintf("%s:%d", remote, flagInitDevnet.BasePort+networks.DnnPortOffset)
		}
	}

	for _, configs := range configs {
		for _, config := range configs {
			config.Accumulate.Network.Subnets = network.Subnets
		}
	}

	if flagInitDevnet.Docker && flagInitDevnet.DnsSuffix != "" {
		for _, remotes := range remotes {
			for i := range remotes {
				remotes[i] += flagInitDevnet.DnsSuffix
			}
		}
	}

	if flagInit.Reset {
		nodeReset()
	}

	if !flagInitDevnet.Compose {
		createInLocalFS(network, configs, nodeDirs, remotes, listens, valKeys)
		return
	}
	// createDockerCompose(cmd, dnRemote, compose)
}

func configDevNet(config *cfg.Config) {
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
}

func initDevNetNode(netType cfg.NetworkType, nodeType cfg.NodeType, name, subnet string, compose *dc.Config) (config *cfg.Config, remote, listen string) {
	config = cfg.Default(cfg.DevNet, netType, nodeType, subnet)
	configDevNet(config)

	if !flagInitDevnet.Docker {
		ip := nextIP()
		return config, ip, ip
	}

	var svc dc.ServiceConfig
	svc.Name = name
	svc.ContainerName = "devnet-" + name
	svc.Image = flagInitDevnet.DockerImage

	if flagInitDevnet.UseVolumes {
		svc.Volumes = []dc.ServiceVolumeConfig{
			{Type: "volume", Source: name, Target: "/node"},
		}
		compose.Volumes[name] = dc.VolumeConfig{}
	} else {
		svc.Volumes = []dc.ServiceVolumeConfig{
			{Type: "bind", Source: "./" + name, Target: "/node"},
		}
	}

	compose.Services = append(compose.Services, svc)
	return config, svc.Name, "0.0.0.0"
}

func createInLocalFS(network *cfg.Network, configs [][]*cfg.Config, nodeDirs, remotes, listens [][]string, valKeys [][]crypto.PrivKey) {
	logger := newLogger()
	netValMap := make(genesis.NetworkValidatorMap)

	var genList []genesis.Bootstrap
	for i, configs := range configs {
		genInit, err := node.Init(node.InitOptions{
			WorkDir:             flagMain.WorkDir,
			Config:              configs,
			NodeDirNames:        nodeDirs[i],
			RemoteIP:            remotes[i],
			ListenIP:            listens[i],
			ValidatorKeys:       valKeys[i],
			NetworkValidatorMap: netValMap,
			Logger:              logger.With("subnet", network.Subnets[i].ID),
		})
		check(err)
		genList = append(genList, genInit)
	}

	// Execute bootstrap after the entire network is known
	for _, genesis := range genList {
		err := genesis.Bootstrap()
		if err != nil {
			panic(fmt.Errorf("could not execute genesis: %v", err))
		}
	}
}

// func createDockerCompose(cmd *cobra.Command, dnRemote []string, compose *dc.Config) {
// 	var svc dc.ServiceConfig
// 	api := fmt.Sprintf("http://%s:%d/v2", dnRemote[0], flagInitDevnet.BasePort+networks.AccApiPortOffset)
// 	svc.Name = "tools"
// 	svc.ContainerName = "devnet-init"
// 	svc.Image = flagInitDevnet.DockerImage
// 	svc.Environment = map[string]*string{"ACC_API": &api}
// 	extras := make(map[string]interface{})
// 	extras["profiles"] = [...]string{"init"}
// 	svc.Extras = extras

// 	svc.Command = dc.ShellCommand{"init", "devnet", "-w", "/nodes", "--docker"}
// 	cmd.Flags().Visit(func(flag *pflag.Flag) {
// 		switch flag.Name {
// 		case "work-dir", "docker", "compose", "reset":
// 			return
// 		}

// 		s := fmt.Sprintf("--%s=%v", flag.Name, flag.Value)
// 		svc.Command = append(svc.Command, s)
// 	})

// 	if flagInitDevnet.UseVolumes {
// 		svc.Volumes = make([]dc.ServiceVolumeConfig, len(compose.Services))
// 		for i, node := range compose.Services {
// 			bits := strings.SplitN(node.Name, "-", 2)
// 			svc.Volumes[i] = dc.ServiceVolumeConfig{Type: "volume", Source: node.Name, Target: path.Join("/nodes", bits[0], "Node"+bits[1])}
// 		}
// 	} else {
// 		svc.Volumes = []dc.ServiceVolumeConfig{
// 			{Type: "bind", Source: ".", Target: "/nodes"},
// 		}
// 	}

// 	compose.Services = append(compose.Services, svc)

// 	dn0svc := compose.Services[0]
// 	dn0svc.Ports = make([]dc.ServicePortConfig, networks.MaxPortOffset+1)
// 	for i := range dn0svc.Ports {
// 		port := uint32(flagInitDevnet.BasePort + i)
// 		dn0svc.Ports[i] = dc.ServicePortConfig{
// 			Mode: "host", Protocol: "tcp", Target: port, Published: port,
// 		}
// 	}

// 	f, err := os.Create(filepath.Join(flagMain.WorkDir, "docker-compose.yml"))
// 	check(err)
// 	defer f.Close()

// 	err = yaml.NewEncoder(f).Encode(compose)
// 	check(err)
// }

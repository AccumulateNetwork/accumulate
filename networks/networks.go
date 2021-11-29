package networks

import (
	"fmt"

	. "github.com/AccumulateNetwork/accumulate/config"
)

const (
	TmP2pPortOffset         = 0
	TmRpcPortOffset         = 1
	TmRpcGrpcPortOffset     = 2
	AccRouterJsonPortOffset = 4
	AccRouterRestPortOffset = 5
	TmPrometheusPortOffset  = 6
)

type Network map[string]*Subnet

type Subnet struct {
	Name        string
	Index       int
	Type        NetworkType
	Port        int
	Directory   string
	Nodes       []Node
	NetworkName string

	Network Network
}

func (s *Subnet) FullName() string { return s.NetworkName + "." + s.Name }

type Node struct {
	IP   string
	Type NodeType
}

var all = Network{}
var nameCount = map[string]int{}

func init() {
	// Ensure 'Directory' must be qualified, e.g. 'DevNet.Directory'
	nameCount["Directory"] = 1

	networks := map[string]Network{
		"TestNet": TestNet,
		"DevNet":  DevNet,
	}

	for _, net := range networks {
		for name, sub := range net {
			nameCount[name]++
			all[sub.NetworkName+"."+name] = sub
			sub.Network = networks[sub.NetworkName]

			if sub.Network == nil {
				panic(fmt.Sprintf("Subnet %q claims it is part of %q but no such network exists", name, sub.NetworkName))
			}

			// If two subnets have the same name, they must be qualified
			switch nameCount[name] {
			case 1:
				all[name] = sub
			case 2:
				delete(all, name)
			}
		}
	}
}

var TestNet = Network{
	"BVC0": {
		Name:        "BVC0",
		NetworkName: "TestNet",
		Type:        BlockValidator,
		Port:        33000,
		Nodes: []Node{
			{"3.140.120.192", Validator},
			{"18.220.147.250", Validator},
			{"52.89.160.158", Validator},
		},
	},
	"BVC1": {
		Name:        "BVC1",
		NetworkName: "TestNet",
		Type:        BlockValidator,
		Port:        33000,
		Nodes: []Node{
			{"65.0.156.146", Validator},
			{"13.234.254.178", Validator},
			{"44.229.57.187", Validator},
		},
	},
	"BVC2": {
		Name:        "BVC2",
		NetworkName: "TestNet",
		Type:        BlockValidator,
		Port:        33000,
		Nodes: []Node{
			{"13.48.159.117", Validator},
			{"16.170.126.251", Validator},
			{"34.214.215.210", Validator},
		},
	},
}

var DevNet = Network{
	"Directory": {
		Name:        "Directory",
		NetworkName: "DevNet",
		Type:        Directory,
		Port:        34000,
		Nodes: []Node{
			{"172.31.4.106", Validator},  // Zion 0
			{"172.31.11.185", Follower},  // Zion 1
			{"172.31.11.104", Validator}, // Yellowstone 0
			{"172.31.13.8", Follower},    // Yellowstone 1
		},
	},
	"Zion": {
		Name:        "Zion",
		NetworkName: "DevNet",
		Index:       0,
		Type:        BlockValidator,
		Port:        33000,
		Directory:   "tcp://localhost:34000",
		Nodes: []Node{
			{"172.31.4.106", Validator},
			{"172.31.11.185", Validator},
		},
	},
	"Yellowstone": {
		Name:        "Yellowstone",
		NetworkName: "DevNet",
		Index:       1,
		Type:        BlockValidator,
		Port:        33000,
		Directory:   "tcp://localhost:34000",
		Nodes: []Node{
			{"172.31.11.104", Validator},
			{"172.31.13.8", Validator},
		},
	},
}

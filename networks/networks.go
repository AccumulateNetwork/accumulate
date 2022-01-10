package networks

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/config"
	"github.com/AccumulateNetwork/accumulate/protocol"
)

const (
	TmP2pPortOffset         = 0
	TmRpcPortOffset         = 1
	TmRpcGrpcPortOffset     = 2
	AccRouterJsonPortOffset = 4
	// AccRouterRestPortOffset = 5
	TmPrometheusPortOffset = 6

	MaxPortOffset = TmPrometheusPortOffset
)

type Network map[string]*Subnet

type Subnet struct {
	Name        string
	Index       int
	Type        config.NetworkType
	Port        int
	Nodes       []Node
	NetworkName string

	Network Network
}

func (s *Subnet) FullName() string { return s.NetworkName + "." + s.Name }

type Node struct {
	IP   string
	Type config.NodeType
}

var all = Network{}
var nameCount = map[string]int{}

func init() {
	// Ensure 'Directory' must be qualified, e.g. 'DevNet.Directory'
	nameCount[protocol.Directory] = 1

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
	protocol.Directory: {
		Name:        protocol.Directory,
		NetworkName: "TestNet",
		Type:        config.Directory,
		Port:        34000,
		Nodes: []Node{
			{"0.bvn0.testnet.accumulatenetwork.io", config.Validator}, // 0-0
			{"1.bvn0.testnet.accumulatenetwork.io", config.Follower},  // 0-1
			{"2.bvn0.testnet.accumulatenetwork.io", config.Follower},  // 0-2
			{"0.bvn1.testnet.accumulatenetwork.io", config.Validator}, // 1-0
			{"1.bvn1.testnet.accumulatenetwork.io", config.Follower},  // 1-1
			{"2.bvn1.testnet.accumulatenetwork.io", config.Follower},  // 1-2
			{"0.bvn2.testnet.accumulatenetwork.io", config.Validator}, // 2-0
			{"1.bvn2.testnet.accumulatenetwork.io", config.Follower},  // 2-1
			{"2.bvn2.testnet.accumulatenetwork.io", config.Follower},  // 2-2
		},
	},
	"BVN0": {
		Name:        "BVN0",
		NetworkName: "TestNet",
		Type:        config.BlockValidator,
		Port:        33000,
		Nodes: []Node{
			{"0.bvn0.testnet.accumulatenetwork.io", config.Validator},
			{"1.bvn0.testnet.accumulatenetwork.io", config.Validator},
			{"2.bvn0.testnet.accumulatenetwork.io", config.Validator},
		},
	},
	"BVN1": {
		Name:        "BVN1",
		NetworkName: "TestNet",
		Type:        config.BlockValidator,
		Port:        33000,
		Nodes: []Node{
			{"0.bvn1.testnet.accumulatenetwork.io", config.Validator},
			{"1.bvn1.testnet.accumulatenetwork.io", config.Validator},
			{"2.bvn1.testnet.accumulatenetwork.io", config.Validator},
		},
	},
	"BVN2": {
		Name:        "BVN2",
		NetworkName: "TestNet",
		Type:        config.BlockValidator,
		Port:        33000,
		Nodes: []Node{
			{"0.bvn2.testnet.accumulatenetwork.io", config.Validator},
			{"1.bvn2.testnet.accumulatenetwork.io", config.Validator},
			{"2.bvn2.testnet.accumulatenetwork.io", config.Validator},
		},
	},
	"SEED": {
		Name:        "SEED",
		NetworkName: "TestNet",
		Type:        config.Directory,
		Port:        26656,
		Nodes: []Node{
			{"0.bvn2.testnet.accumulatenetwork.io", config.Seed},
		},
	},
}

var DevNet = Network{
	protocol.Directory: {
		Name:        protocol.Directory,
		NetworkName: "DevNet",
		Type:        config.Directory,
		Port:        34000,
		Nodes: []Node{
			{"172.31.4.106", config.Validator},  // Zion 0
			{"172.31.11.185", config.Follower},  // Zion 1
			{"172.31.11.104", config.Validator}, // Yellowstone 0
			{"172.31.13.8", config.Follower},    // Yellowstone 1
		},
	},
	"Zion": {
		Name:        "Zion",
		NetworkName: "DevNet",
		Index:       0,
		Type:        config.BlockValidator,
		Port:        33000,
		Nodes: []Node{
			{"172.31.4.106", config.Validator},
			{"172.31.11.185", config.Validator},
		},
	},
	"Yellowstone": {
		Name:        "Yellowstone",
		NetworkName: "DevNet",
		Index:       1,
		Type:        config.BlockValidator,
		Port:        33000,
		Nodes: []Node{
			{"172.31.11.104", config.Validator},
			{"172.31.13.8", config.Validator},
		},
	},
}

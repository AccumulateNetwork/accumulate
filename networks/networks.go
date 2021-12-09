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
	IP   string // TODO this can also be a hostname, but renaming it will produce a big PR
	Type NodeType
}

var all = Network{}
var nameCount = map[string]int{}

var Networks = map[string]Network{
	"TestNet": TestNet,
	"DevNet":  DevNet,
}

func init() {
	// Ensure 'Directory' must be qualified, e.g. 'DevNet.Directory'
	nameCount["Directory"] = 1

	for _, net := range Networks {
		for name, sub := range net {
			nameCount[name]++
			all[sub.NetworkName+"."+name] = sub
			sub.Network = Networks[sub.NetworkName]

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
			{"0.bvn0.testnet.accumulatenetwork.io", Validator},
			{"1.bvn0.testnet.accumulatenetwork.io", Validator},
			{"2.bvn0.testnet.accumulatenetwork.io", Validator},
		},
	},
	"BVC1": {
		Name:        "BVC1",
		NetworkName: "TestNet",
		Type:        BlockValidator,
		Port:        33000,
		Nodes: []Node{
			{"0.bvn1.testnet.accumulatenetwork.io", Validator},
			{"1.bvn1.testnet.accumulatenetwork.io", Validator},
			{"2.bvn1.testnet.accumulatenetwork.io", Validator},
		},
	},
	"BVC2": {
		Name:        "BVC2",
		NetworkName: "TestNet",
		Type:        BlockValidator,
		Port:        33000,
		Nodes: []Node{
			{"0.bvn2.testnet.accumulatenetwork.io", Validator},
			{"1.bvn2.testnet.accumulatenetwork.io", Validator},
			{"2.bvn2.testnet.accumulatenetwork.io", Validator},
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
			{"0.zion.devnet.accumulatenetwork.io", Validator},
			{"1.zion.devnet.accumulatenetwork.io", Follower},
			{"0.yellowstone.devnet.accumulatenetwork.io", Validator},
			{"1.yellowstone.devnet.accumulatenetwork.io", Follower},
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
			{"0.zion.devnet.accumulatenetwork.io", Validator},
			{"1.zion.devnet.accumulatenetwork.io", Validator},
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
			{"0.yellowstone.devnet.accumulatenetwork.io", Validator},
			{"1.yellowstone.devnet.accumulatenetwork.io", Validator},
		},
	},
}

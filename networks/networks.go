package networks

import (
	"fmt"

	. "github.com/AccumulateNetwork/accumulate/config"
)

type Network map[string]*Subnet

type Subnet struct {
	Name  string
	Index int
	Type  NetworkType
	Port  int
	Nodes []Node
}

type Node struct {
	IP   string
	Type NodeType
}

var All = func() Network {
	all := Network{}
	for _, net := range []Network{TestNet, DevNet, OGTestNet, Local} {
		for name, sub := range net {
			if all[name] != nil {
				panic(fmt.Errorf("networks: redefined %q", name))
			}
			all[name] = sub
		}
	}
	return all
}()

var TestNet = Network{
	"BVC0": {
		Name: "BVC0",
		Type: BVC,
		Port: 33000,
		Nodes: []Node{
			{"3.140.120.192", Validator},
			{"18.220.147.250", Validator},
			{"52.89.160.158", Validator},
		},
	},
	"BVC1": {
		Name: "BVC1",
		Type: BVC,
		Port: 33000,
		Nodes: []Node{
			{"65.0.156.146", Validator},
			{"13.234.254.178", Validator},
			{"44.229.57.187", Validator},
		},
	},
	"BVC2": {
		Name: "BVC2",
		Type: BVC,
		Port: 33000,
		Nodes: []Node{
			{"13.48.159.117", Validator},
			{"16.170.126.251", Validator},
			{"34.214.215.210", Validator},
		},
	},
}

var DevNet = Network{
	"Zion": {
		Name:  "Zion",
		Index: 0,
		Type:  BVC,
		Port:  33000,
		Nodes: []Node{
			{"172.31.4.106", Validator},
			{"172.31.11.185", Validator},
		},
	},
	"Yellowstone": {
		Name:  "Yellowstone",
		Index: 1,
		Type:  BVC,
		Port:  33000,
		Nodes: []Node{
			{"172.31.11.104", Validator},
			{"172.31.13.8", Validator},
		},
	},
}

var OGTestNet = Network{
	"Arches": {
		Name:  "Arches",
		Index: 0,
		Type:  BVC,
		Port:  33000,
		Nodes: []Node{
			{"13.51.10.110", Validator},
			{"13.232.230.216", Validator},
		},
	},
	"AmericanSamoa": {
		Name:  "AmericanSamoa",
		Index: 1,
		Type:  BVC,
		Port:  33000,
		Nodes: []Node{
			{"18.221.39.36", Validator},
			{"44.236.45.58", Validator},
		},
	},
	"EastXeons": {
		Name:  "EastXeons",
		Index: 2,
		Type:  BVC,
		Port:  33000,
		Nodes: []Node{
			{"18.119.26.7", Validator},
			{"18.119.149.208", Validator},
		},
	},
	"EastXeons-DC": {
		Name:  "EastXeons-DC",
		Index: -1,
		Type:  DC,
		Port:  33100,
		Nodes: []Node{
			{"18.119.26.7", Validator},
			{"18.119.149.208", Validator},
		},
	},
}

var Local = Network{
	"Badlands": {
		Name: "Badlands",
		Type: BVC,
		Port: 35550,
		Nodes: []Node{
			{"127.0.0.1", Validator},
		},
	},
	"BigBend": {
		Name: "BigBend",
		Type: BVC,
		Port: 26656,
		Nodes: []Node{
			{"127.0.1.1", Validator},
			{"127.0.1.2", Validator},
			{"127.0.1.3", Validator},
		},
	},
}

package networks

import (
	"strconv"

	. "github.com/AccumulateNetwork/accumulated/config"
)

type Network struct {
	Name  string
	Type  NetworkType
	Port  int
	Nodes []Node
}

type Node struct {
	IP   string
	Type NodeType
}

var Networks = []Network{
	{
		Name: "Arches",
		Type: BVC,
		Port: 33000,
		Nodes: []Node{
			{"13.51.10.110", Validator},
			{"13.232.230.216", Validator},
		},
	},
	{
		Name: "AmericanSamoa",
		Type: BVC,
		Port: 33000,
		Nodes: []Node{
			{"18.221.39.36", Validator},
			{"44.236.45.58", Validator},
		},
	},
	{
		Name: "Badlands",
		Type: BVC,
		Port: 35550,
		Nodes: []Node{
			{"127.0.0.1", Validator},
		},
	},
	{
		Name: "EastXeons",
		Type: BVC,
		Port: 33000,
		Nodes: []Node{
			{"18.119.26.7", Validator},
			{"18.119.149.208", Validator},
		},
	},
	{
		Name: "EastXeons-DC",
		Type: DC,
		Port: 33100,
		Nodes: []Node{
			{"18.119.26.7", Validator},
			{"18.119.149.208", Validator},
		},
	},
	{
		Name: "Localhost",
		Type: BVC,
		Port: 26656,
		Nodes: []Node{
			{"127.0.1.1", Validator},
			{"127.0.1.2", Validator},
			{"127.0.1.3", Validator},
		},
	},
}

func IndexOf(name string) int {
	// If name is a number between 0 and len(Networks), accept that
	n, err := strconv.ParseInt(name, 10, 32)
	if err == nil && 0 <= n && int(n) < len(Networks) {
		return int(n)
	}

	for i, net := range Networks {
		if net.Name == name {
			return i
		}
	}
	return -1
}

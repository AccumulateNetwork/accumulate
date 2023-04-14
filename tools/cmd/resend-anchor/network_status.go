// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"net"
	stdurl "net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/rpc/client/http"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var listNodesCmd = &cobra.Command{
	Use:   "list-nodes [seeds]",
	Short: "List the nodes of the network",
	Run: func(_ *cobra.Command, args []string) {
		_, nodes := walkNetwork(args)
		printNodes(nodes)
	},
}

func init() {
	cmd.AddCommand(listNodesCmd)
}

type NodeData struct {
	Hostname    string                  `json:"hostname"`
	BasePort    uint64                  `json:"basePort"`
	ValidatorID [20]byte                `json:"validatorID"`
	DnNodeID    [20]byte                `json:"dnNodeID"`
	DnnStatus   *coretypes.ResultStatus `json:"dnnStatus"`
	BvnNodeID   [20]byte                `json:"bvnNodeID"`
	BvnnStatus  *coretypes.ResultStatus `json:"bvnnStatus"`
	Info        *protocol.ValidatorInfo `json:"info"`
}

func (n NodeData) AccumulateAPI(offset uint64) *client.Client {
	c, err := client.New(fmt.Sprintf("http://%s:%d", n.Hostname, n.BasePort+offset+uint64(config.PortOffsetAccumulateApi)))
	checkf(err, "client for %s", n.Hostname)
	return c
}

func (n NodeData) AccumulateAPIForPartition(id string) *client.Client {
	// Get the port offset for the partition
	var offset uint64
	if strings.EqualFold(protocol.Directory, id) {
		offset = config.PortOffsetDirectory
	} else {
		offset = config.PortOffsetBlockValidator
	}

	return n.AccumulateAPI(offset)
}

func (n NodeData) AccumulateAPIForUrl(u *url.URL) *client.Client {
	// Get the partition ID
	id, ok := protocol.ParsePartitionUrl(u)
	if !ok {
		fatalf("%v is not a partition", u)
	}

	return n.AccumulateAPIForPartition(id)
}

func walkNetwork(addrs []string) (*core.GlobalValues, []*NodeData) {
	fmt.Println("Walking the network")
	var nodes []*NodeData
	byNodeId := map[[20]byte]*NodeData{}

	hostname, port := parseAddr(addrs[0])
	if hostname == "" {
		os.Exit(1)
	}
	addr := fmt.Sprintf("http://%s:%d", hostname, port+uint64(config.PortOffsetAccumulateApi))
	if flag.Debug {
		fmt.Printf("Accumulate describe %s\n", addr)
	}
	acc, err := client.New(addr)
	checkf(err, "new DNN Acc client")

	describe, err := acc.Describe(context.Background())
	checkf(err, "DNN Acc describe")

	valInfo := map[[20]byte]*protocol.ValidatorInfo{}
	for _, val := range describe.Values.Network.Validators {
		id := val.PublicKeyHash[:20]
		valInfo[*(*[20]byte)(id)] = val
	}

	skipped := map[string]bool{}
	for len(addrs) > 0 {
		var next []string
		for _, addr := range addrs {
			hostname, port := parseAddr(addr)
			if hostname == "" {
				continue
			}
			if skipped[hostname] {
				continue
			}

			addr := fmt.Sprintf("http://%s:%d", hostname, port+uint64(config.PortOffsetTendermintRpc))
			if flag.Debug {
				fmt.Printf("Tendermint status %s\n", addr)
			}
			tm, err := http.NewWithTimeout(addr, addr+"/websocket", 1)
			if warnf(err, "new DNN TM client") {
				continue
			}

			status, err := tm.Status(context.Background())
			if err != nil {
				fmt.Printf("  Skipping %s: %v\n", hostname, err)
				skipped[hostname] = true
				continue
			}

			nodeId, err := hex.DecodeString(string(status.NodeInfo.DefaultNodeID))
			if warnf(err, "parse node ID") {
				continue
			}

			valId, err := hex.DecodeString(status.ValidatorInfo.Address.String())
			if warnf(err, "parse validator ID") {
				continue
			}

			_, ok := byNodeId[*(*[20]byte)(nodeId)]
			if ok {
				continue
			}

			fmt.Printf("  Found %s\n", hostname)
			n := new(NodeData)
			n.Hostname = hostname
			n.BasePort = port
			n.ValidatorID = *(*[20]byte)(valId)
			n.Info = valInfo[n.ValidatorID]
			n.DnNodeID = *(*[20]byte)(nodeId)
			n.DnnStatus = status
			delete(valInfo, n.ValidatorID)

			nodes = append(nodes, n)
			byNodeId[n.DnNodeID] = n

			if flag.Debug {
				fmt.Printf("Tendermint net-info %s\n", addr)
			}
			netInfo, err := tm.NetInfo(context.Background())
			if warnf(err, "DNN TM net info") {
				continue
			}

			for _, peer := range netInfo.Peers {
				id, err := hex.DecodeString(string(peer.NodeInfo.DefaultNodeID))
				if warnf(err, "parse peer ID") {
					continue
				}
				if byNodeId[*(*[20]byte)(id)] != nil {
					continue
				}

				addr := peer.NodeInfo.ListenAddr
				if !strings.Contains(addr, "://") {
					addr = "tcp://" + addr
				}
				u, err := stdurl.Parse(addr)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Invalid peer: bad listen address: %q", peer.NodeInfo.ListenAddr)
					continue
				}
				// fmt.Printf("  %s has peer %s\n", hostname, peer.URL)
				next = append(next, "tcp://"+peer.RemoteIP+":"+u.Port())
			}

			addr = fmt.Sprintf("http://%s:%d", hostname, port+uint64(config.PortOffsetTendermintRpc+config.PortOffsetBlockValidator))
			if flag.Debug {
				fmt.Printf("Tendermint status %s\n", addr)
			}
			tm, err = http.New(addr, addr+"/websocket")
			if warnf(err, "new BVNN TM client") {
				continue
			}

			status, err = tm.Status(context.Background())
			if warnf(err, "BVNN TM status") {
				continue
			}

			nodeId, err = hex.DecodeString(string(status.NodeInfo.DefaultNodeID))
			if warnf(err, "parse node ID") {
				continue
			}

			n.BvnNodeID = *(*[20]byte)(nodeId)
			n.BvnnStatus = status
			byNodeId[n.BvnNodeID] = n
		}
		addrs = next
	}

	for id, val := range valInfo {
		n := new(NodeData)
		n.ValidatorID = id
		n.Info = val
		nodes = append(nodes, n)
	}

	sort.Slice(nodes, func(i, j int) bool {
		return bytes.Compare(nodes[i].ValidatorID[:], nodes[j].ValidatorID[:]) < 0
	})
	return &describe.Values, nodes
}

func parseAddr(s string) (string, uint64) {
	u, err := stdurl.Parse(s)
	checkf(err, "parse url %q", s)

	port, err := strconv.ParseUint(u.Port(), 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Error: invalid peer: %s\n", s)
		return "", 0
	}
	port -= uint64(config.PortOffsetTendermintP2P + config.PortOffsetDirectory)

	hostname := u.Hostname()
	if net.ParseIP(hostname) != nil {
		return hostname, port
	}

	ip, err := net.LookupIP(hostname)
	if err != nil {
		return hostname, port
	}

	hostname = ip[0].String()
	return hostname, port
}

func printNodes(nodes []*NodeData) {
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 1, ' ', 0)
	defer tw.Flush()

	fmt.Fprintf(tw, "Validator\tKey\tNode\tAddress\tActive\n")
	for _, n := range nodes {
		var addr string
		if n.Hostname != "" {
			addr = fmt.Sprintf("%s:%d", n.Hostname, n.BasePort)
		}
		var active []string
		if n.Info != nil {
			for _, part := range n.Info.Partitions {
				active = append(active, part.ID)
			}
		}

		var key []byte
		if n.Info != nil {
			key = n.Info.PublicKey[:4]
		}

		fmt.Fprintf(tw, "%x\t%x\t%x\t%s\t%s\n", sliceOrNil(n.ValidatorID), key, sliceOrNil(n.DnNodeID), addr, strings.Join(active, ","))
	}
}

func sliceOrNil(id [20]byte) []byte {
	if id == ([20]byte{}) {
		return nil
	}
	return id[:]
}

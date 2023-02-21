// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"log"
	stdurl "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var healCmd = &cobra.Command{
	Use:  "heal [network]",
	Args: cobra.MinimumNArgs(1),
	Run:  heal,
}

func init() {
	cmd.AddCommand(healCmd)
	healCmd.Flags().BoolVar(&healFlag.Continuous, "continuous", false, "Run healing in a loop every second")
}

var healFlag = struct {
	Continuous bool
}{}

func heal(_ *cobra.Command, args []string) {
	netu, err := stdurl.Parse(args[0])
	checkf(err, "network url")
	netp, err := strconv.ParseUint(netu.Port(), 10, 64)
	checkf(err, "network url port")
	netc, err := client.New(fmt.Sprintf("http://%s:%d", netu.Hostname(), netp+uint64(config.PortOffsetAccumulateApi)+config.PortOffsetDirectory))
	checkf(err, "network client")
	g, nodes := walkNetwork(args)

	partNodes := map[string][]*NodeData{}
	for _, part := range g.Network.Partitions {
		var pn []*NodeData
		for _, node := range nodes {
			if node.Hostname != "" && node.Info != nil && node.Info.IsActiveOn(part.ID) {
				pn = append(pn, node)
			}
		}
		if uint64(len(pn)) < g.ValidatorThreshold(part.ID) {
			fatalf("Error: insufficient nodes for %s; threshold is %d but I only found the address of %d nodes", part.ID, g.ValidatorThreshold(part.ID), len(pn))
		}
		partNodes[strings.ToLower(part.ID)] = pn
	}

heal:
	// Heal BVN -> DN
	for _, part := range g.Network.Partitions {
		if part.Type != protocol.PartitionTypeBlockValidator {
			continue
		}

		partUrl := protocol.PartitionUrl(part.ID)
		ledger := getLedger(netc, partUrl)

		for _, txid := range ledger.Anchor(protocol.DnUrl()).Pending {
			healTx(g, partNodes, netc, protocol.DnUrl(), partUrl, txid)
		}
	}

	// Heal DN -> BVN, DN -> DN
	{
		ledger := getLedger(netc, protocol.DnUrl())

		for _, part := range g.Network.Partitions {
			partUrl := protocol.PartitionUrl(part.ID)
			for _, txid := range ledger.Anchor(partUrl).Pending {
				healTx(g, partNodes, netc, partUrl, protocol.DnUrl(), txid)
			}
		}
	}

	// Heal continuously?
	if healFlag.Continuous {
		time.Sleep(time.Second)
		goto heal
	}
}

func getLedger(c *client.Client, part *url.URL) *protocol.AnchorLedger {
	ledger := new(protocol.AnchorLedger)
	res := new(api.ChainQueryResponse)
	res.Data = ledger
	req := new(api.GeneralQuery)
	req.Url = part.JoinPath(protocol.AnchorPool)
	err := c.RequestAPIv2(context.Background(), "query", req, res)
	checkf(err, "query %s anchor ledger", part)
	return ledger
}

func healTx(g *core.GlobalValues, nodes map[string][]*NodeData, netClient *client.Client, srcUrl, dstUrl *url.URL, txid *url.TxID) {
	dstId, _ := protocol.ParsePartitionUrl(dstUrl)
	srcId, _ := protocol.ParsePartitionUrl(srcUrl)

	// Query the transaction
	res, err := netClient.QueryTx(context.Background(), &api.TxnQuery{TxIdUrl: txid})
	if err != nil {
		log.Printf("Failed to query %v: %v\n", txid, err)
		return
	}

	// Check if there are already enough transactions
	if uint64(len(res.Status.AnchorSigners)) >= g.ValidatorThreshold(srcId) {
		return // Already have enough signers
	}

	fmt.Printf("Healing anchor %v\n", txid)

	// Mark which nodes have signed
	signed := map[[32]byte]bool{}
	for _, s := range res.Status.AnchorSigners {
		signed[*(*[32]byte)(s)] = true
	}

	// Make a client for the destination
	dstClient := nodes[strings.ToLower(dstId)][0].AccumulateAPIForUrl(dstUrl)

	// Get a signature from each node that hasn't signed
	for _, node := range nodes[strings.ToLower(srcId)] {
		if signed[*(*[32]byte)(node.Info.PublicKey)] {
			continue
		}

		// Make a client for the source
		srcClient := node.AccumulateAPIForUrl(srcUrl)

		// Query and execute the anchor
		querySynthAndExecute(srcClient, dstClient, srcUrl, dstUrl, res.Status.SequenceNumber)
	}
}

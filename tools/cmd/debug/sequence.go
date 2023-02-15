// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdSequence = &cobra.Command{
	Use:   "sequence [server]",
	Short: "Debug synthetic and anchor sequencing",
	Run:   sequence,
}

func init() {
	cmd.AddCommand(cmdSequence)
}

func sequence(_ *cobra.Command, args []string) {
	c, err := client.New(args[0])
	check(err)

	desc, err := c.Describe(context.Background())
	check(err)

	anchors := map[string]*protocol.AnchorLedger{}
	synths := map[string]*protocol.SyntheticLedger{}
	bad := map[Dir]bool{}
	for _, part := range desc.Values.Network.Partitions {
		// Get anchor ledger
		req := new(api.GeneralQuery)
		req.Url = protocol.PartitionUrl(part.ID).JoinPath(protocol.AnchorPool)
		anchor := new(protocol.AnchorLedger)
		res := new(api.ChainQueryResponse)
		res.Data = anchor
		err = c.RequestAPIv2(context.Background(), "query", req, res)
		check(err)
		anchors[part.ID] = anchor

		// Get synthetic ledger
		req.Url = protocol.PartitionUrl(part.ID).JoinPath(protocol.Synthetic)
		synth := new(protocol.SyntheticLedger)
		res.Data = synth
		err = c.RequestAPIv2(context.Background(), "query", req, res)
		check(err)
		synths[part.ID] = synth

		// Check pending and received vs delivered
		for _, src := range anchor.Sequence {
			checkSequence1(part, src, bad, "anchors")
		}

		for _, src := range synth.Sequence {
			checkSequence1(part, src, bad, "synthetic transactions")
		}
	}

	// Check produced vs received
	for i, a := range desc.Values.Network.Partitions {
		for _, b := range desc.Values.Network.Partitions[i:] {
			checkSequence2(a, b, bad, "anchors",
				anchors[a.ID].Anchor(protocol.PartitionUrl(b.ID)),
				anchors[b.ID].Anchor(protocol.PartitionUrl(a.ID)),
			)
			checkSequence2(a, b, bad, "synthetic transactions",
				synths[a.ID].Partition(protocol.PartitionUrl(b.ID)),
				synths[b.ID].Partition(protocol.PartitionUrl(a.ID)),
			)
		}
	}

	for _, a := range desc.Values.Network.Partitions {
		for _, b := range desc.Values.Network.Partitions {
			if !bad[Dir{From: a.ID, To: b.ID}] {
				color.Green("✔ %s → %s\n", a.ID, b.ID)
			}
		}
	}
}

type Dir struct {
	From, To string
}

func checkSequence2(a, b *protocol.PartitionInfo, bad map[Dir]bool, kind string, ab, ba *protocol.PartitionSyntheticLedger) {
	if ab.Produced > ba.Received {
		color.Red("🗴 %s → %s has %d unreceived %s\n", a.ID, b.ID, ab.Produced-ba.Received, kind)
		bad[Dir{From: a.ID, To: b.ID}] = true
	}
	if ba.Produced > ab.Received {
		color.Red("🗴 %s → %s has %d unreceived %s\n", b.ID, a.ID, ba.Produced-ab.Received, kind)
		bad[Dir{From: b.ID, To: a.ID}] = true
	}
}

func checkSequence1(dst *protocol.PartitionInfo, src *protocol.PartitionSyntheticLedger, bad map[Dir]bool, kind string) {
	id, _ := protocol.ParsePartitionUrl(src.Url)
	if len(src.Pending) > 0 {
		color.Red("🗴 %s → %s has %d pending %s\n", id, dst.ID, len(src.Pending), kind)
		bad[Dir{From: id, To: dst.ID}] = true
	}
	if src.Received > src.Delivered {
		color.Red("🗴 %s → %s has %d unprocessed %s\n", id, dst.ID, src.Received-src.Delivered, kind)
		bad[Dir{From: id, To: dst.ID}] = true
	}
}

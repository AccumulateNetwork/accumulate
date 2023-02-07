// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdHealSynth = &cobra.Command{
	Use:   "heal-synth [server]",
	Short: "Fixup synthetic transactions",
	Run:   healSynth,
}

func init() {
	cmd.AddCommand(cmdHealSynth)
}

func healSynth(_ *cobra.Command, args []string) {
	c, err := client.New(args[0])
	check(err)

	desc, err := c.Describe(context.Background())
	check(err)

	synths := map[string]*protocol.SyntheticLedger{}
	for _, part := range desc.Values.Network.Partitions {
		// Get synthetic ledger
		req := new(api.GeneralQuery)
		req.Url = protocol.PartitionUrl(part.ID).JoinPath(protocol.Synthetic)
		synth := new(protocol.SyntheticLedger)
		res := new(api.ChainQueryResponse)
		res.Data = synth
		err = c.RequestAPIv2(context.Background(), "query", req, res)
		check(err)
		synths[part.ID] = synth

		for _, src := range synth.Sequence {
			for _, txid := range src.Pending {
				req.Url = txid.AsUrl()
				res := new(api.TransactionQueryResponse)
				err = c.RequestAPIv2(context.Background(), "query", req, res)
				check(err)

				fmt.Printf("Resubmitting %v\n", txid)
				xreq := new(api.ExecuteRequest)
				xreq.Envelope = new(messaging.Envelope)
				xreq.Envelope.Transaction = []*protocol.Transaction{res.Transaction}
				for _, sig := range res.Signatures {
					sig, ok := sig.(*protocol.PartitionSignature)
					if ok {
						xreq.Envelope.Signatures = []protocol.Signature{sig}
					}
				}
				xres, err := c.ExecuteDirect(context.Background(), xreq)
				check(err)
				if xres.Message != "" {
					fmt.Fprintf(os.Stderr, "Warning: %s\n", xres.Message)
				}
			}
		}
	}

	// // Check produced vs received
	// for i, a := range desc.Values.Network.Partitions {
	// 	for _, b := range desc.Values.Network.Partitions[i+1:] {
	// 		checkSequence2(a, b, bad, "synthetic transactions",
	// 			synths[a.ID].Partition(protocol.PartitionUrl(b.ID)),
	// 			synths[b.ID].Partition(protocol.PartitionUrl(a.ID)),
	// 		)
	// 	}
	// }
}

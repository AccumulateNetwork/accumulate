// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
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
	c.DebugRequest = true

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

	// Check produced vs received
	for i, a := range desc.Values.Network.Partitions {
		for _, b := range desc.Values.Network.Partitions[i:] {
			ab := synths[a.ID].Partition(protocol.PartitionUrl(b.ID))
			ba := synths[b.ID].Partition(protocol.PartitionUrl(a.ID))

			for i := ba.Received + 1; i <= ab.Produced; i++ {
				resubmitByNumber(desc, c, a.ID, b.ID, i, false)
			}
			if a == b {
				continue
			}
			for i := ab.Received + 1; i <= ba.Produced; i++ {
				resubmitByNumber(desc, c, b.ID, a.ID, i, false)
			}
		}
	}
}

func resubmitByNumber(desc *api.DescriptionResponse, c *client.Client, source, destination string, number uint64, anchor bool) {
	// Get a client for the destination partition
	var d *client.Client
	for _, p := range desc.Network.Partitions {
		if !strings.EqualFold(p.Id, destination) {
			continue
		}
		if len(p.Nodes) == 0 {
			fatalf("no nodes for %v", p.Id)
		}
		u, err := url.Parse(p.Nodes[0].Address)
		check(err)
		port, err := strconv.ParseUint(u.Port(), 10, 16)
		check(err)
		d, err = client.New(fmt.Sprintf("http://%s:%d", u.Hostname(), port+config.PortOffsetAccumulateApi.GetEnumValue()))
		check(err)
	}

	// Query the synthetic transaction
	req := new(api.SyntheticTransactionRequest)
	req.Source = protocol.PartitionUrl(source)
	req.Destination = protocol.PartitionUrl(destination)
	req.SequenceNumber = number
	req.Anchor = anchor
	res, err := c.QuerySynth(context.Background(), req)
	check(err)

	// Submit the synthetic transaction directly to the destination partition
	fmt.Printf("Resubmitting %v\n", res.Txid)
	xreq := new(api.ExecuteRequest)
	xreq.Envelope = new(messaging.Envelope)
	xreq.Envelope.Transaction = []*protocol.Transaction{res.Transaction}
	xreq.Envelope.Signatures = res.Signatures
	xres, err := d.ExecuteLocal(context.Background(), xreq)
	check(err)
	if xres.Message != "" {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", xres.Message)
	}
}

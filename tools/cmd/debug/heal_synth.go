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

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	v2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
)

var cmdHeal = &cobra.Command{
	Use: "heal",
}

var cmdHealSynth = &cobra.Command{
	Use:   "synth [network] [server]",
	Short: "Fixup synthetic transactions",
	Args:  cobra.ExactArgs(2),
	Run:   healSynth,
}

var flagHealSynth = struct {
	Peer string
}{}

func init() {
	cmd.AddCommand(cmdHeal)
	cmdHeal.AddCommand(cmdHealSynth)
	cmdHealSynth.Flags().StringVar(&flagHealSynth.Peer, "peer", "", "Query a specific peer")
}

func healSynth(_ *cobra.Command, args []string) {
	testing.EnableDebugFeatures()
	c, err := client.New(args[1])
	check(err)

	desc, err := c.Describe(context.Background())
	check(err)

	node, err := p2p.New(p2p.Options{
		Network:        args[0],
		BootstrapPeers: api.BootstrapServers,
	})
	check(err)
	defer func() { _ = node.Close() }()

	fmt.Printf("We are %v\n", node.ID())

	router := new(routing.MessageRouter)
	c2 := &message.Client{
		Transport: &message.RoutedTransport{
			Network: args[0],
			Dialer:  node.DialNetwork(),
			Router:  router,
		},
	}
	router.Router, err = routing.NewStaticRouter(desc.Values.Routing, nil)
	check(err)

	synths := map[string]*protocol.SyntheticLedger{}
	for _, part := range desc.Values.Network.Partitions {
		// Get synthetic ledger
		req := new(v2.GeneralQuery)
		req.Url = protocol.PartitionUrl(part.ID).JoinPath(protocol.Synthetic)
		synth := new(protocol.SyntheticLedger)
		res := new(v2.ChainQueryResponse)
		res.Data = synth
		err = c.RequestAPIv2(context.Background(), "query", req, res)
		check(err)
		synths[part.ID] = synth

		for _, src := range synth.Sequence {
			for _, txid := range src.Pending {
				req.Url = txid.AsUrl()
				res := new(v2.TransactionQueryResponse)
				err = c.RequestAPIv2(context.Background(), "query", req, res)
				check(err)

				xreq := new(v2.ExecuteRequest)
				xreq.Envelope = new(messaging.Envelope)
				xreq.Envelope.Transaction = []*protocol.Transaction{res.Transaction}
				var partSig *protocol.PartitionSignature
				for _, sig := range res.Signatures {
					sig, ok := sig.(*protocol.PartitionSignature)
					if ok {
						partSig = sig
						xreq.Envelope.Signatures = []protocol.Signature{sig}
					}
				}

				p := c2.Private()
				if flagHealSynth.Peer != "" {
					pid, err := peer.Decode(flagHealSynth.Peer)
					check(err)
					p = c2.ForPeer(pid).Private()
				}

				// Get a signature
				r, err := p.Sequence(context.Background(), partSig.SourceNetwork.JoinPath(protocol.Synthetic), partSig.DestinationNetwork, partSig.SequenceNumber)
				check(err)
				var note string
				for _, sigs := range r.Signatures.Records {
					for _, sig := range sigs.Signatures.Records {
						if sig, ok := sig.Message.(*messaging.SignatureMessage); ok {
							switch sig := sig.Signature.(type) {
							case protocol.KeySignature:
								xreq.Envelope.Signatures = append(xreq.Envelope.Signatures, sig)
							case *protocol.ReceiptSignature:
								if !res.Status.GotDirectoryReceipt {
									note = " with DN receipt"
									xreq.Envelope.Signatures = append(xreq.Envelope.Signatures, sig)
								}
							}
						}
					}
				}

				fmt.Printf("Resubmitting %v%s\n", txid, note)
				// b, _ := json.Marshal(xreq.Envelope)
				// fmt.Printf("%s\n", b)
				// return

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

func resubmitByNumber(desc *v2.DescriptionResponse, c *client.Client, source, destination string, number uint64, anchor bool) {
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
	req := new(v2.SyntheticTransactionRequest)
	req.Source = protocol.PartitionUrl(source)
	req.Destination = protocol.PartitionUrl(destination)
	req.SequenceNumber = number
	req.Anchor = anchor
	res, err := c.QuerySynth(context.Background(), req)
	check(err)

	// Submit the synthetic transaction directly to the destination partition
	fmt.Printf("Resubmitting %v\n", res.Txid)
	xreq := new(v2.ExecuteRequest)
	xreq.Envelope = new(messaging.Envelope)
	xreq.Envelope.Transaction = []*protocol.Transaction{res.Transaction}
	xreq.Envelope.Signatures = res.Signatures
	xres, err := d.ExecuteLocal(context.Background(), xreq)
	check(err)
	if xres.Message != "" {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", xres.Message)
	}
}

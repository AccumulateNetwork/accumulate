// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdSequence = &cobra.Command{
	Use:   "sequence [server]",
	Short: "Debug synthetic and anchor sequencing",
	Run:   sequence,
}

func init() {
	cmd.AddCommand(cmdSequence)
	cmdSequence.Flags().BoolVarP(&verbose, "verbose", "v", false, "More verbose outputt")
}

func sequence(cmd *cobra.Command, args []string) {
	ctx, cancel, _ := api.ContextWithBatchData(cmd.Context())
	defer cancel()

	c := jsonrpc.NewClient(api.ResolveWellKnownEndpoint(args[0]))
	ns, err := c.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	check(err)
	Q := api.Querier2{Querier: c}

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
			Dialer:  &hackDialer{c, node.DialNetwork(), map[string]peer.ID{}},
			Router:  router,
		},
	}
	router.Router, err = routing.NewStaticRouter(ns.Routing, nil)
	check(err)

	anchors := map[string]*protocol.AnchorLedger{}
	synths := map[string]*protocol.SyntheticLedger{}
	bad := map[Dir]bool{}
	for _, part := range ns.Network.Partitions {
		// Get anchor ledger
		dst := protocol.PartitionUrl(part.ID)
		var anchor *protocol.AnchorLedger
		_, err = Q.QueryAccountAs(ctx, dst.JoinPath(protocol.AnchorPool), nil, &anchor)
		check(err)
		anchors[part.ID] = anchor

		// Get synthetic ledger
		var synth *protocol.SyntheticLedger
		_, err = Q.QueryAccountAs(ctx, dst.JoinPath(protocol.Synthetic), nil, &synth)
		check(err)
		synths[part.ID] = synth

		// Check pending and received vs delivered
		for _, src := range anchor.Sequence {
			ids, _ := findPendingAnchors(ctx, c2, Q, src.Url, dst, verbose)
			src.Pending = append(src.Pending, ids...)

			checkSequence1(part, src, bad, "anchors")
		}

		for _, src := range synth.Sequence {
			checkSequence1(part, src, bad, "synthetic transactions")
		}
	}

	// Check produced vs received
	for i, a := range ns.Network.Partitions {
		for _, b := range ns.Network.Partitions[i:] {
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

	for _, a := range ns.Network.Partitions {
		for _, b := range ns.Network.Partitions {
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
	if a == b {
		return
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
		if verbose {
			for _, id := range src.Pending {
				fmt.Printf("  %v\n", id)
			}
		}
	}
	if src.Received > src.Delivered {
		color.Red("🗴 %s → %s has %d unprocessed %s\n", id, dst.ID, src.Received-src.Delivered, kind)
		bad[Dir{From: id, To: dst.ID}] = true
	}
}

func findPendingAnchors(ctx context.Context, C *message.Client, Q api.Querier2, src, dst *url.URL, resolve bool) ([]*url.TxID, map[[32]byte]*protocol.Transaction) {
	srcId, _ := protocol.ParsePartitionUrl(src)
	dstId, _ := protocol.ParsePartitionUrl(dst)

	// Check how many have been received
	var dstLedger *protocol.AnchorLedger
	_, err := Q.QueryAccountAs(ctx, dst.JoinPath(protocol.AnchorPool), nil, &dstLedger)
	checkf(err, "query %v → %v anchor ledger", srcId, dstId)
	dstSrcLedger := dstLedger.Partition(src)
	received := dstSrcLedger.Received

	// Check how many should have been sent
	srcDstChain, err := Q.QueryChain(ctx, src.JoinPath(protocol.AnchorPool), &api.ChainQuery{Name: "anchor-sequence"})
	checkf(err, "query %v anchor sequence chain", srcId)

	if received >= srcDstChain.Count-1 {
		return nil, nil
	}

	// Non-verbose mode doesn't care about the actual IDs
	if !resolve {
		return make([]*url.TxID, srcDstChain.Count-received-1), nil
	}

	var ids []*url.TxID
	txns := map[[32]byte]*protocol.Transaction{}
	for i := received + 1; i <= srcDstChain.Count; i++ {
		msg, err := C.Private().Sequence(ctx, src.JoinPath(protocol.AnchorPool), dst, i)
		checkf(err, "query %v → %v anchor #%d", srcId, dstId, i)
		ids = append(ids, msg.ID)

		txn := msg.Message.(*messaging.TransactionMessage)
		txns[txn.Hash()] = txn.Transaction
	}
	return ids, txns
}

type hackDialer struct {
	api  api.NodeService
	node message.Dialer
	good map[string]peer.ID
}

func (h *hackDialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	// Have we found a good peer?
	if id, ok := h.good[addr.String()]; ok {
		s, err := h.dial(ctx, addr, id)
		if err == nil {
			return s, nil
		}
		fmt.Printf("%v failed with %v\n", id, err)
		delete(h.good, addr.String())
	}

	// Unpack the service address
	network, peer, service, err := api.UnpackAddress(addr)
	if err != nil {
		return nil, err
	}

	// If it specifies a node, do nothing
	if peer != "" {
		return h.node.Dial(ctx, addr)
	}

	// Use the API to find a node
	nodes, err := h.api.FindService(ctx, api.FindServiceOptions{Network: network, Service: service})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("locate nodes for %v: %w", addr, err)
	}
	if len(nodes) == 0 {
		return nil, errors.NoPeer.WithFormat("cannot locate a peer for %v", addr)
	}

	// Try all the nodes
	for _, n := range nodes {
		s, err := h.dial(ctx, addr, n.PeerID)
		if err == nil {
			h.good[addr.String()] = n.PeerID
			return s, nil
		}
		fmt.Printf("%v failed with %v\n", n.PeerID, err)
	}
	return nil, errors.NoPeer.WithFormat("no peers are responding for %v", addr)
}

func (h *hackDialer) dial(ctx context.Context, addr multiaddr.Multiaddr, peer peer.ID) (message.Stream, error) {
	c, err := multiaddr.NewComponent("p2p", peer.String())
	if err != nil {
		return nil, err
	}
	addr = addr.Encapsulate(c)
	return h.node.Dial(ctx, addr)
}

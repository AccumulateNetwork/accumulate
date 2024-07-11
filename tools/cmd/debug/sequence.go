// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/exp/apiutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/healing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
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
	cmdSequence.Flags().BoolVarP(&verbose, "verbose", "v", false, "More verbose output")
	cmdSequence.Flags().BoolVar(&debug, "debug", false, "Debug network requests")
	cmdSequence.Flags().StringVar(&cachedScan, "cached-scan", "", "A cached network scan")
	cmdSequence.Flags().StringVar(&only, "only", "", "Only scan anchors or synthetic transactions")
	cmdSequence.Flags().DurationVar(&flagMaxResponseAge, "max-response-age", flagMaxResponseAge, "Maximum age of a response before it is considered too stale to use")
}

func sequence(cmd *cobra.Command, args []string) {
	ctx, cancel, _ := api.ContextWithBatchData(cmd.Context())
	defer cancel()

	c := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(args[0], "v3"))
	c.Client.Timeout = time.Hour
	c.Debug = debug
	Q := api.Querier2{Querier: c}

	ni, err := c.NodeInfo(ctx, api.NodeInfoOptions{})
	check(err)

	node, err := p2p.New(p2p.Options{
		Network:        ni.Network,
		BootstrapPeers: bootstrap,
	})
	check(err)
	defer func() { _ = node.Close() }()

	fmt.Printf("We are %v\n", node.ID())

	var net *healing.NetworkInfo
	if cachedScan == "" {
		f := filepath.Join(cacheDir, strings.ToLower(ni.Network)+".json")
		if st, err := os.Stat(f); err == nil && !st.IsDir() {
			slog.Info("Detected network scan", "file", f)
			cachedScan = f
		} else if !errors.Is(err, fs.ErrNotExist) {
			check(err)
		}
	}
	if cachedScan != "" {
		data, err := os.ReadFile(cachedScan)
		check(err)
		check(json.Unmarshal(data, &net))
	}

	router := new(routing.MessageRouter)
	c2 := &message.Client{
		Transport: &message.RoutedTransport{
			Network: ni.Network,
			Dialer: &apiutil.StaticDialer{
				Scan:   net,
				Nodes:  c,
				Dialer: node.DialNetwork(),
			},
			Router: router,
			Debug:  debug,
		},
	}
	Q.Querier = c2

	fmt.Println("Network status")
	ns, err := c.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	check(err)
	router.Router = routing.NewRouter(routing.RouterOptions{Initial: ns.Routing})

	var scanSynth, scanAnchors bool
	switch only {
	case "":
		scanSynth = true
		scanAnchors = true
	case "a", "anchor", "anchors", "anchoring":
		scanAnchors = true
	case "s", "synth", "synthetic":
		scanSynth = true
	default:
		fatalf("invalid --only %q", only)
	}

	anchors := map[string]*protocol.AnchorLedger{}
	synths := map[string]*protocol.SyntheticLedger{}
	bad := map[Dir]bool{}
	for _, part := range ns.Network.Partitions {
		fmt.Println("Query", part.ID)

		// Get anchor ledger
		dst := protocol.PartitionUrl(part.ID)
		anchor := getAccount[*protocol.AnchorLedger](ctx, Q, dst.JoinPath(protocol.AnchorPool))
		anchors[part.ID] = anchor

		// Get synthetic ledger
		synth := getAccount[*protocol.SyntheticLedger](ctx, Q, dst.JoinPath(protocol.Synthetic))
		synths[part.ID] = synth

		// Check pending and received vs delivered
		if scanAnchors {
			for _, src := range anchor.Sequence {
				ids, _ := findPendingAnchors(ctx, c2, Q, net, src.Url, dst, verbose)
				src.Pending = append(src.Pending, ids...)

				checkSequence1(part, src, bad, "anchors")
			}
		}

		if scanSynth {
			for _, src := range synth.Sequence {
				checkSequence1(part, src, bad, "synthetic transactions")
			}
		}
	}

	// Check produced vs received
	for i, a := range ns.Network.Partitions {
		for _, b := range ns.Network.Partitions[i:] {
			if scanAnchors {
				checkSequence2(a, b, bad, "anchors",
					anchors[a.ID].Anchor(protocol.PartitionUrl(b.ID)),
					anchors[b.ID].Anchor(protocol.PartitionUrl(a.ID)),
				)
			}
			if scanSynth {
				checkSequence2(a, b, bad, "synthetic transactions",
					synths[a.ID].Partition(protocol.PartitionUrl(b.ID)),
					synths[b.ID].Partition(protocol.PartitionUrl(a.ID)),
				)
			}
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
		color.Red("🗴 %s → %s has %d unreceived %s (%d → %d)\n", a.ID, b.ID, ab.Produced-ba.Received, kind, ba.Received, ab.Produced)
		bad[Dir{From: a.ID, To: b.ID}] = true
	}
	if a == b {
		return
	}
	if ba.Produced > ab.Received {
		color.Red("🗴 %s → %s has %d unreceived %s (%d → %d)\n", b.ID, a.ID, ba.Produced-ab.Received, kind, ab.Received, ba.Produced)
		bad[Dir{From: b.ID, To: a.ID}] = true
	}
}

func checkSequence1(dst *protocol.PartitionInfo, src *protocol.PartitionSyntheticLedger, bad map[Dir]bool, kind string) {
	id, _ := protocol.ParsePartitionUrl(src.Url)
	if len(src.Pending) > 0 {
		color.Red("🗴 %s → %s has %d pending %s (from %d)\n", id, dst.ID, len(src.Pending), kind, src.Delivered+1)
		bad[Dir{From: id, To: dst.ID}] = true
		if verbose {
			for _, id := range src.Pending {
				fmt.Printf("  %v\n", id)
			}
		}
	}
	if src.Received > src.Delivered {
		color.Red("🗴 %s → %s has %d unprocessed %s (%d → %d)\n", id, dst.ID, src.Received-src.Delivered, kind, src.Delivered, src.Received)
		bad[Dir{From: id, To: dst.ID}] = true
	}
}

func findPendingAnchors(ctx context.Context, C *message.Client, Q api.Querier2, net *healing.NetworkInfo, src, dst *url.URL, resolve bool) ([]*url.TxID, map[[32]byte]*protocol.Transaction) {
	srcId, _ := protocol.ParsePartitionUrl(src)
	dstId, _ := protocol.ParsePartitionUrl(dst)

	// Check how many have been received
	dstLedger := getAccount[*protocol.AnchorLedger](ctx, Q, dst.JoinPath(protocol.AnchorPool))
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
		var msg *api.MessageRecord[messaging.Message]
		if net == nil {
			slog.Info("Checking anchor", "source", src, "destination", dst, "number", i, "remaining", srcDstChain.Count-i)
			msg, err = C.Private().Sequence(ctx, src.JoinPath(protocol.AnchorPool), dst, i, private.SequenceOptions{})
			checkf(err, "query %v → %v anchor #%d", srcId, dstId, i)
		} else {
			for _, peer := range net.Peers[strings.ToLower(srcId)] {
				ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
				slog.Info("Checking anchor", "source", src, "destination", dst, "number", i, "remaining", srcDstChain.Count-i, "peer", peer.ID)
				msg, err = C.ForPeer(peer.ID).Private().Sequence(ctx, src.JoinPath(protocol.AnchorPool), dst, i, private.SequenceOptions{})
				if err == nil {
					break
				}
				slog.Error("Failed to check anchor", "source", src, "destination", dst, "number", i, "remaining", srcDstChain.Count-i, "peer", peer.ID, "error", err)
			}
			if msg == nil {
				fatalf("query %v → %v anchor #%d failed", srcId, dstId, i)
			}
		}

		ids = append(ids, msg.ID)

		txn := msg.Message.(*messaging.TransactionMessage)
		txns[txn.Hash()] = txn.Transaction
	}
	return ids, txns
}

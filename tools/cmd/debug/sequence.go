// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdSequence = &cobra.Command{
	Use:   "sequence [server]",
	Short: "Debug synthetic and anchor sequencing",
	Args:  cobra.ExactArgs(1),
	Run:   sequence,
}

func init() {
	cmd.AddCommand(cmdSequence)
	cmdSequence.Flags().BoolVarP(&verbose, "verbose", "v", false, "More verbose output")
	cmdSequence.Flags().StringVar(&only, "only", "", "Only scan anchors or synthetic transactions")
	healerFlags(cmdSequence)
}

func sequence(cmd *cobra.Command, args []string) {
	ctx, cancel, _ := api.ContextWithBatchData(cmd.Context())
	defer cancel()

	h := new(healer)
	h.setup(ctx, args[0])

	fmt.Println("Network status")
	ns, err := h.C2.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: protocol.Directory})
	check(err)

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
		anchor := getAccount[*protocol.AnchorLedger](h, dst.JoinPath(protocol.AnchorPool))
		// Check if the ledger is nil before proceeding
		if anchor == nil {
			slog.WarnContext(h.ctx, "Unable to process anchors due to nil ledger", "destination", dst)
			continue
		}
		anchors[part.ID] = anchor

		// Get synthetic ledger
		synth := getAccount[*protocol.SyntheticLedger](h, dst.JoinPath(protocol.Synthetic))
		// Check if the ledger is nil before proceeding
		if synth == nil {
			slog.WarnContext(h.ctx, "Unable to process synthetic transactions due to nil ledger", "destination", dst)
			continue
		}
		synths[part.ID] = synth

		// Check pending and received vs delivered
		if scanAnchors {
			for _, src := range anchor.Sequence {
				ids, _ := h.findPendingAnchors(src.Url, dst, verbose)
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

func (h *healer) findPendingAnchors(src, dst *url.URL, resolve bool) ([]*url.TxID, map[[32]byte]*protocol.Transaction) {
	srcId, _ := protocol.ParsePartitionUrl(src)
	dstId, _ := protocol.ParsePartitionUrl(dst)

	// Check how many have been received
	dstLedger := getAccount[*protocol.AnchorLedger](h, dst.JoinPath(protocol.AnchorPool))
	// Check if the ledger is nil before proceeding
	if dstLedger == nil {
		slog.WarnContext(h.ctx, "Unable to process anchors due to nil ledger", "destination", dst)
		return nil, nil
	}
	
	dstSrcLedger := dstLedger.Partition(src)
	received := dstSrcLedger.Received

	// Check how many should have been sent
	srcDstChain, err := h.tryEach().QueryChain(h.ctx, src.JoinPath(protocol.AnchorPool), &api.ChainQuery{Name: "anchor-sequence"})
	if err != nil {
		// Check if it's a PeerUnavailableError
		if _, ok := err.(*PeerUnavailableError); ok {
			slog.WarnContext(h.ctx, "Unable to query anchor sequence chain due to peer unavailability", "source", srcId)
			return nil, nil
		}
		// For other errors, use the original behavior
		checkf(err, "query %v anchor sequence chain", srcId)
	}

	if received >= srcDstChain.Count-1 {
		return nil, nil
	}

	// Calculate missing anchor information
	firstMissingHeight := received + 1
	totalMissingAnchors := srcDstChain.Count - received - 1

	// Limit the number of anchors to process to 5 per pass
	maxAnchorsToProcess := uint64(5)
	pendingCount := totalMissingAnchors
	processLimit := pendingCount
	if pendingCount > maxAnchorsToProcess {
		processLimit = maxAnchorsToProcess
	}
	
	// Single comprehensive log for the pending anchors
	logAttrs := []any{
		"node", h.network,
		"source", srcId,
		"destination", dstId,
		"current_height", received,
		"first_missing_height", firstMissingHeight,
		"missing_anchors", totalMissingAnchors,
	}
	
	if processLimit < pendingCount {
		logAttrs = append(logAttrs, 
			"processing", processLimit,
			"status", "limited")
	} else {
		logAttrs = append(logAttrs, "status", "processing_all")
	}
	
	slog.InfoContext(h.ctx, "Anchor healing status", logAttrs...)

	// Non-verbose mode doesn't care about the actual IDs
	if !resolve {
		return make([]*url.TxID, processLimit), nil
	}

	var ids []*url.TxID
	txns := map[[32]byte]*protocol.Transaction{}
	// Process only up to the limit (starting with the oldest)
	processedCount := uint64(0)
	for i := received + 1; i <= srcDstChain.Count && processedCount < maxAnchorsToProcess; i++ {
		var msg *api.MessageRecord[messaging.Message]
		if h.net == nil {
			slog.DebugContext(h.ctx, "Checking anchor", "source", src, "destination", dst, "number", i, "remaining", srcDstChain.Count-i)
			msg, err = h.C2.Private().Sequence(h.ctx, src.JoinPath(protocol.AnchorPool), dst, i, private.SequenceOptions{})
			if err != nil {
				// Check if it's a PeerUnavailableError
				if _, ok := err.(*PeerUnavailableError); ok {
					slog.WarnContext(h.ctx, "Unable to query sequence due to peer unavailability", 
						"source", srcId, "destination", dstId, "number", i)
					continue
				}
				// For other errors, use the original behavior
				checkf(err, "query %v → %v anchor #%d", srcId, dstId, i)
			}
		} else {
			for _, peer := range h.net.Peers[strings.ToLower(srcId)] {
				ctx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
				defer cancel()
				slog.DebugContext(h.ctx, "Checking anchor", "source", src, "destination", dst, "number", i, "remaining", srcDstChain.Count-i, "peer", peer.ID)
				msg, err = h.C2.ForPeer(peer.ID).Private().Sequence(ctx, src.JoinPath(protocol.AnchorPool), dst, i, private.SequenceOptions{})
				if err == nil {
					break
				}
				slog.WarnContext(h.ctx, "Failed to check anchor", "source", src, "destination", dst, "number", i, "peer", peer.ID, "error", err)
			}
			if msg == nil {
				slog.WarnContext(h.ctx, "Unable to query anchor due to peer unavailability", 
					"source", srcId, "destination", dstId, "number", i)
				continue
			}
		}

		ids = append(ids, msg.ID)
		processedCount++

		txn := msg.Message.(*messaging.TransactionMessage)
		txns[txn.Hash()] = txn.Transaction
	}
	return ids, txns
}

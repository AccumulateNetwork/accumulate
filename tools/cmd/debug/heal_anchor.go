// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"log/slog"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/healing"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdHealAnchor = &cobra.Command{
	Use:   "anchor [network]",
	Short: "Heal anchors",
	Args:  cobra.MaximumNArgs(1),
	Run:   healAnchor,
}

func init() {
	cmdHeal.AddCommand(cmdHealAnchor)
}

func healAnchor(_ *cobra.Command, args []string) {
	lightDb = ""
	h := &healer{
		healSingle: func(h *healer, src, dst *protocol.PartitionInfo, num uint64, txid *url.TxID) {
			h.healSingleAnchor(src.ID, dst.ID, num, txid, nil)
		},
		healSequence: func(h *healer, src, dst *protocol.PartitionInfo) {
			// Skip BVN to BVN anchors
			if src.Type != protocol.PartitionTypeDirectory && dst.Type != protocol.PartitionTypeDirectory {
				return
			}

			srcUrl := protocol.PartitionUrl(src.ID)
			dstUrl := protocol.PartitionUrl(dst.ID)

		pullAgain:
			dstLedger := getAccount[*protocol.AnchorLedger](h, dstUrl.JoinPath(protocol.AnchorPool))
			// Check if the ledger is nil before proceeding
			if dstLedger == nil {
				slog.WarnContext(h.ctx, "Unable to process anchors due to nil ledger", "destination", dstUrl)
				return
			}
			src2dst := dstLedger.Partition(srcUrl)
			
			// Get the chain info to determine missing anchors
			srcDstChain, err := h.tryEach().QueryChain(h.ctx, srcUrl.JoinPath(protocol.AnchorPool), &v3.ChainQuery{Name: "anchor-sequence"})
			if err != nil {
				// Check if it's a PeerUnavailableError
				if _, ok := err.(*PeerUnavailableError); ok {
					slog.WarnContext(h.ctx, "Unable to query anchor sequence chain due to peer unavailability", "source", src.ID)
					return
				}
				// For other errors, use the original behavior
				checkf(err, "query %v anchor sequence chain", src.ID)
			}
			
			// Calculate missing anchor information
			firstMissingHeight := src2dst.Delivered + 1
			totalMissingAnchors := srcDstChain.Count - src2dst.Delivered
			
			// Skip if no anchors are missing
			if totalMissingAnchors <= 0 {
				return
			}
			
			// Single comprehensive log for the healing process
			slog.InfoContext(h.ctx, "Anchor healing status",
				"node", h.network,
				"source", src.ID,
				"destination", dst.ID,
				"current_height", src2dst.Delivered,
				"first_missing_height", firstMissingHeight,
				"missing_anchors", totalMissingAnchors,
				"status", "starting")

			ids, txns := h.findPendingAnchors(srcUrl, dstUrl, true)

			var all []*url.TxID
			all = append(all, src2dst.Pending...)
			all = append(all, ids...)

			for i, txid := range all {
				select {
				case <-h.ctx.Done():
					return
				default:
				}
				if h.healSingleAnchor(src.ID, dst.ID, src2dst.Delivered+1+uint64(i), txid, txns) {
					// If it was already delivered, recheck the ledgers
					goto pullAgain
				}
			}
		},
	}

	h.heal(args)
}

func (h *healer) healSingleAnchor(srcId, dstId string, seqNum uint64, txid *url.TxID, txns map[[32]byte]*protocol.Transaction) bool {
	// We'll log the result after the healing attempt, not before
	var count int
	var status string
	
retry:
	err := healing.HealAnchor(h.ctx, healing.HealAnchorArgs{
		Client:  h.C2.ForAddress(nil),
		Querier: h.tryEach(),
		NetInfo: h.net,
		Known:   txns,
		Pretend: pretend,
		Wait:    waitForTxn,
		Submit: func(m ...messaging.Message) error {
			select {
			case h.submit <- m:
				return nil
			case <-h.ctx.Done():
				return errors.NotReady.With("canceled")
			}
		},
	}, healing.SequencedInfo{
		Source:      srcId,
		Destination: dstId,
		Number:      seqNum,
		ID:          txid,
	})
	
	// Determine the status based on the error
	if err == nil {
		status = "healed"
	} else if errors.Is(err, errors.Delivered) {
		status = "already_delivered"
		return true
	} else if !errors.Is(err, healing.ErrRetry) {
		status = "failed"
		slog.InfoContext(h.ctx, "Anchor healing status", 
			"node", h.network,
			"source", srcId,
			"destination", dstId,
			"height", seqNum,
			"status", status,
			"error", err)
		return false
	} else {
		// It's a retry error
		count++
		if count >= 3 {
			status = "skipped_after_retries"
			slog.InfoContext(h.ctx, "Anchor healing status", 
				"node", h.network,
				"source", srcId,
				"destination", dstId,
				"height", seqNum,
				"status", status,
				"attempts", count)
			return false
		}
		
		status = "retrying"
		slog.InfoContext(h.ctx, "Anchor healing status", 
			"node", h.network,
			"source", srcId,
			"destination", dstId,
			"height", seqNum,
			"status", status,
			"attempts", count)
		goto retry
	}
	
	// Log the final status if we haven't already
	if status == "healed" {
		slog.InfoContext(h.ctx, "Anchor healing status", 
			"node", h.network,
			"source", srcId,
			"destination", dstId,
			"height", seqNum,
			"status", status)
	}
	
	return false
}

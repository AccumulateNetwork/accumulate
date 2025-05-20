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
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdHealAnchor = &cobra.Command{
	Use:   "anchor [network] [txid or part→part (optional) [sequence number (optional)]]",
	Short: "Heal anchoring",
	Args:  cobra.RangeArgs(1, 3),
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
			src2dst := dstLedger.Partition(srcUrl)

			ids, txns := h.findPendingAnchors(srcUrl, dstUrl, true)

			var all []*url.TxID
			all = append(all, src2dst.Pending...)
			if len(all) > 20 {
				all = all[:20]
			}
			if len(ids) > 20 {
				ids = ids[:20]
			}
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
	var count int
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
	if err == nil {
		return false
	}
	if errors.Is(err, errors.Delivered) {
		return true
	}
	if !errors.Is(err, healing.ErrRetry) {
		slog.Error("Failed to heal", "source", srcId, "destination", dstId, "number", seqNum, "error", err)
		return false
	}

	count++
	if count >= 10 {
		slog.Error("Anchor still pending, skipping", "attempts", count)
		return false
	}
	slog.Error("Anchor still pending, retrying", "attempts", count)
	goto retry
}

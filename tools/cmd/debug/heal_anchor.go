// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"errors"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/healing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slog"
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

			dstLedger := getAccount[*protocol.AnchorLedger](h, dstUrl.JoinPath(protocol.AnchorPool))
			src2dst := dstLedger.Partition(srcUrl)

			ids, txns := findPendingAnchors(h.ctx, h.C2, h.tryEach(), h.net, srcUrl, dstUrl, true)

			var all []*url.TxID
			all = append(all, src2dst.Pending...)
			all = append(all, ids...)

			for i, txid := range all {
				h.healSingleAnchor(src.ID, dst.ID, src2dst.Delivered+1+uint64(i), txid, txns)
			}
		},
	}

	h.heal(args)
}

func (h *healer) healSingleAnchor(srcId, dstId string, seqNum uint64, txid *url.TxID, txns map[[32]byte]*protocol.Transaction) {
	var count int
retry:
	err := healing.HealAnchor(h.ctx, healing.HealAnchorArgs{
		Client:    h.C2.ForAddress(nil),
		Querier:   h.tryEach(),
		Submitter: h.C2,
		NetInfo:   h.net,
		Known:     txns,
		Pretend:   pretend,
		Wait:      waitForTxn,
	}, healing.SequencedInfo{
		Source:      srcId,
		Destination: dstId,
		Number:      seqNum,
		ID:          txid,
	})
	if err == nil {
		return
	}
	if !errors.Is(err, healing.ErrRetry) {
		slog.Error("Failed to heal", "source", srcId, "destination", dstId, "number", seqNum, "error", err)
		return
	}

	count++
	if count >= 10 {
		slog.Error("Anchor still pending, skipping", "attempts", count)
		return
	}
	slog.Error("Anchor still pending, retrying", "attempts", count)
	goto retry
}

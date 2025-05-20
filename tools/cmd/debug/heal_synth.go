// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"log/slog"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/healing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdHealSynth = &cobra.Command{
	Use:   "synth [network] [txid or part→part (optional) [sequence number (optional)]]",
	Short: "Fixup synthetic transactions",
	Args:  cobra.RangeArgs(1, 3),
	Run:   healSynth,
}

func init() {
	cmdHeal.AddCommand(cmdHealSynth)
	cmdHealSynth.Flags().DurationVar(&healSinceDuration, "since", 48*time.Hour, "How far back in time to heal (0 for forever)")
}

func healSynth(cmd *cobra.Command, args []string) {
	if !cmd.Flags().Changed("wait") {
		waitForTxn = true
	}

	h := &healer{
		healSingle: func(h *healer, src, dst *protocol.PartitionInfo, num uint64, txid *url.TxID) {
			srcUrl := protocol.PartitionUrl(src.ID)
			dstUrl := protocol.PartitionUrl(dst.ID)

			// Pull chains
			pullSynthDirChains(h)
			pullSynthSrcChains(h, srcUrl)

			// Pull accounts
			pullSynthLedger(h, srcUrl)
			pullSynthLedger(h, dstUrl)

			// Heal
			healSingleSynth(h, src.ID, dst.ID, num, txid)
		},
		healSequence: func(h *healer, src, dst *protocol.PartitionInfo) {
			srcUrl := protocol.PartitionUrl(src.ID)
			dstUrl := protocol.PartitionUrl(dst.ID)

			// Pull chains
			pullSynthDirChains(h)
			pullSynthSrcChains(h, srcUrl)

			// Pull accounts
		pullAgain:
			ab := pullSynthLedger(h, srcUrl).Partition(dstUrl)
			ba := pullSynthLedger(h, dstUrl).Partition(srcUrl)

			// Heal
			for i := uint64(0); i+ba.Delivered < ab.Produced; i++ {
				if i > 20 {
					break
				}
				select {
				case <-h.ctx.Done():
					return
				default:
				}
				var id *url.TxID
				if i < uint64(len(ba.Pending)) {
					id = ba.Pending[i]
				}
				if healSingleSynth(h, src.ID, dst.ID, ba.Delivered+i+1, id) {
					// If it was already delivered, recheck the ledgers
					goto pullAgain
				}
			}
		},
	}

	h.heal(args)
}

func healSingleSynth(h *healer, source, destination string, number uint64, id *url.TxID) bool {
	var count int
retry:
	err := h.HealSynthetic(h.ctx, healing.HealSyntheticArgs{
		Client:    h.C2.ForAddress(nil),
		Querier:   h.C2,
		Submitter: h.C2,
		NetInfo:   h.net,
		Light:     h.light,
		Pretend:   pretend,
		Wait:      waitForTxn,

		// If an attempt fails, use the next anchor
		SkipAnchors: count,
	}, healing.SequencedInfo{
		Source:      source,
		Destination: destination,
		Number:      number,
		ID:          id,
	})
	if err == nil {
		return false
	}
	if errors.Is(err, errors.Delivered) {
		return true
	}
	if !errors.Is(err, healing.ErrRetry) {
		slog.Error("Failed to heal", "source", source, "destination", destination, "number", number, "error", err)
		return false
	}

	count++
	if count >= 3 {
		slog.Error("Message still pending, skipping", "attempts", count)
		return false
	}
	slog.Error("Message still pending, trying next anchor", "attempts", count)
	goto retry
}

func pullSynthDirChains(h *healer) {
	ctx, cancel, _ := api.ContextWithBatchData(h.ctx)
	defer cancel()

	check(h.light.PullAccount(ctx, protocol.DnUrl().JoinPath(protocol.Network)))

	check(h.light.PullAccountWithChains(ctx, protocol.DnUrl().JoinPath(protocol.Ledger), includeRootChain))
	check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.Ledger)))

	check(h.light.PullAccountWithChains(ctx, protocol.DnUrl().JoinPath(protocol.AnchorPool), func(c *api.ChainRecord) bool {
		return c.Type == merkle.ChainTypeAnchor || c.IndexOf != nil && c.IndexOf.Type == merkle.ChainTypeAnchor
	}))
	check(h.light.IndexAccountChains(ctx, protocol.DnUrl().JoinPath(protocol.AnchorPool)))
}

func pullSynthSrcChains(h *healer, part *url.URL) {
	ctx, cancel, _ := api.ContextWithBatchData(h.ctx)
	defer cancel()

	check(h.light.PullAccountWithChains(ctx, part.JoinPath(protocol.Ledger), includeRootChain))
	check(h.light.IndexAccountChains(ctx, part.JoinPath(protocol.Ledger)))

	check(h.light.PullAccountWithChains(ctx, part.JoinPath(protocol.Synthetic), func(c *api.ChainRecord) bool {
		return strings.HasPrefix(c.Name, "synthetic-sequence") || c.Name == "main" || c.Name == "main-index"
	}))
	check(h.light.IndexAccountChains(ctx, part.JoinPath(protocol.Synthetic)))
}

func pullSynthLedger(h *healer, part *url.URL) *protocol.SyntheticLedger {
	check(h.light.PullAccountWithChains(h.ctx, part.JoinPath(protocol.Synthetic), func(cr *api.ChainRecord) bool { return false }))

	batch := h.light.OpenDB(false)
	defer batch.Discard()

	var ledger *protocol.SyntheticLedger
	check(batch.Account(part.JoinPath(protocol.Synthetic)).Main().GetAs(&ledger))
	return ledger
}

func includeRootChain(c *api.ChainRecord) bool {
	switch c.Name {
	case "root", "root-index":
		return true
	}
	return false
}

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

			// Calculate missing synthetic transactions
			totalMissingTxs := uint64(0)
			if ab.Produced > ba.Delivered {
				totalMissingTxs = ab.Produced - ba.Delivered
			}
			
			// Skip if no synthetic transactions are missing
			if totalMissingTxs <= 0 {
				return
			}
			
			// Limit the number of synthetic transactions to process to 5 per partition pair (source/destination) per pass
			maxTxsToProcess := uint64(5)
			processLimit := totalMissingTxs
			if totalMissingTxs > maxTxsToProcess {
				processLimit = maxTxsToProcess
			}
			
			// Log the synthetic healing status
			var status string
			if processLimit < totalMissingTxs {
				status = "limited_per_pair"
			} else {
				status = "processing_all"
			}
			
			slog.InfoContext(h.ctx, "Synthetic healing status for partition pair",
				"node", h.network,
				"source", src.ID,
				"destination", dst.ID,
				"current_height", ba.Delivered,
				"first_missing_height", ba.Delivered + 1,
				"missing_txs", totalMissingTxs,
				"processing", processLimit,
				"status", status)

			// Heal only up to the limit
			for i := uint64(0); i < processLimit; i++ {
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
	var status string
	
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
	
	// Determine the status based on the error
	if err == nil {
		status = "healed"
		slog.InfoContext(h.ctx, "Synthetic healing status for transaction", 
			"node", h.network,
			"source", source,
			"destination", destination,
			"height", number,
			"status", status)
		return false
	} else if errors.Is(err, errors.Delivered) {
		status = "already_delivered"
		slog.InfoContext(h.ctx, "Synthetic healing status for transaction", 
			"node", h.network,
			"source", source,
			"destination", destination,
			"height", number,
			"status", status)
		return true
	} else if !errors.Is(err, healing.ErrRetry) {
		status = "failed"
		slog.InfoContext(h.ctx, "Synthetic healing status for transaction", 
			"node", h.network,
			"source", source,
			"destination", destination,
			"height", number,
			"status", status,
			"error", err)
		return false
	}
	
	// It's a retry error
	count++
	if count >= 3 {
		status = "skipped_after_retries"
		slog.InfoContext(h.ctx, "Synthetic healing status for transaction", 
			"node", h.network,
			"source", source,
			"destination", destination,
			"height", number,
			"status", status,
			"attempts", count)
		return false
	}
	
	status = "retrying"
	slog.InfoContext(h.ctx, "Synthetic healing status for transaction", 
		"node", h.network,
		"source", source,
		"destination", destination,
		"height", number,
		"status", status,
		"attempts", count)
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
	err := h.light.PullAccountWithChains(h.ctx, part.JoinPath(protocol.Synthetic), func(cr *api.ChainRecord) bool { return false })
	if err != nil {
		slog.WarnContext(h.ctx, "Failed to pull synthetic ledger", "part", part, "error", err)
		return nil
	}

	batch := h.light.OpenDB(false)
	defer batch.Discard()

	var ledger *protocol.SyntheticLedger
	err = batch.Account(part.JoinPath(protocol.Synthetic)).Main().GetAs(&ledger)
	if err != nil {
		slog.WarnContext(h.ctx, "Failed to get synthetic ledger", "part", part, "error", err)
		return nil
	}
	return ledger
}

func includeRootChain(c *api.ChainRecord) bool {
	switch c.Name {
	case "root", "root-index":
		return true
	}
	return false
}

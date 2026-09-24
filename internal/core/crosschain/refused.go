// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"log/slog"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// refusalReporter is a dispatcher that tells its caller what a destination
// refused. The node's dispatcher is one; the conductor registers with it when
// it starts.
type refusalReporter interface {
	OnRefused(func(partition string, env *messaging.Envelope, err error))
}

// refused is told of every envelope a destination refused. A heal goes to
// this partition, and a refused heal is a failed heal: the requester recorded
// the span as asked when the source answered, before the destination had
// judged the envelope, so it forgets the entries the envelope carried and the
// next activation asks for them again (healing.md, "A refused heal is a
// failed heal"; #4426). Refusals of envelopes to other partitions are the
// dispatcher's to count; nothing here asked for them.
func (c *Conductor) refused(partition string, env *messaging.Envelope, err error) {
	if env == nil || !strings.EqualFold(partition, c.Partition.ID) {
		return
	}
	numbers := map[string][]uint64{}
	streams := map[string]execute.StreamID{}
	for _, m := range env.Messages {
		var inner messaging.Message
		var ledger string
		switch m := m.(type) {
		case *messaging.SyntheticMessage:
			inner, ledger = m.Message, protocol.Synthetic
		case *messaging.BadSyntheticMessage:
			inner, ledger = m.Message, protocol.Synthetic
		case *messaging.BlockAnchor:
			inner, ledger = m.Anchor, protocol.AnchorPool
		default:
			continue
		}
		seq, ok := inner.(*messaging.SequencedMessage)
		if !ok || seq.Source == nil {
			continue
		}
		stream := execute.StreamID{Ledger: c.Url(ledger), Source: seq.Source.RootIdentity()}
		k := streamKey(stream)
		streams[k] = stream
		numbers[k] = append(numbers[k], seq.Number)
	}
	for k, stream := range streams {
		// Only what was asked for is a heal. The Directory's own anchor to
		// itself, or a synthetic to this partition, refused after a committee
		// change, was never asked for: the dispatcher has counted it, and it
		// is not a heal refusal (#4426 review F1).
		if !c.requester.unask(stream, numbers[k]) {
			continue
		}
		mHealRequests.WithLabelValues("refused", c.Partition.ID, partitionLabel(stream.Source)).Inc()
		slog.Error("Destination refused healed entries; they will be asked for again", "module", "conductor",
			"source", stream.Source, "destination", c.Url(), "ledger", stream.Ledger, "entries", len(numbers[k]), "error", err)
	}
}

// unask drops every asked span that holds one of numbers, so the stream's
// next activation asks for them again instead of waiting out patience. It
// reports whether it dropped any: whether the refused entries were asked for.
func (r *healRequester) unask(stream execute.StreamID, numbers []uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := streamKey(stream)
	if len(r.asks[k]) == 0 {
		return false // and the map may be nil: nothing has been asked yet
	}
	kept := r.asks[k][:0]
	for _, a := range r.asks[k] {
		holds := false
		for _, n := range numbers {
			if a.first <= n && n <= a.last {
				holds = true
				break
			}
		}
		if !holds {
			kept = append(kept, a)
		}
	}
	dropped := len(kept) < len(r.asks[k])
	clear(r.asks[k][len(kept):])
	r.asks[k] = kept
	return dropped
}

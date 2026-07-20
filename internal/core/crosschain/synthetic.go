// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// syntheticHealWindow is the jitter/back-off window for receiver-pull synthetic
// healing. When a synthetic stream stalls, each validator waits a random delay
// in [0, syntheticHealWindow) before requesting the missing message, and backs
// off for the same window between requests. ~10 block times at ~1s/block, so
// the first requester's re-submission is delivered — advancing Delivered on
// every validator — before most others fire. See issue #4064.
const syntheticHealWindow = 10 * time.Second

// mHeals counts cross-partition messages that were healed. For synthetics the
// heal is receiver-side (remote = the source partition the message was pulled
// from); for anchors it is source-side (remote = the destination partition the
// anchor was re-submitted to). Exposed on the node's Prometheus endpoint.
var mHeals = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "crosschain",
	Name:      "heals_total",
	Help:      "Number of cross-partition messages healed (re-requested or re-submitted)",
}, []string{"type", "partition", "remote"})

// synthHealEntry tracks this node's back-off state for a single stalled inbound
// synthetic stream.
type synthHealEntry struct {
	want    uint64    // the sequence number this stream is stalled on
	fireAt  time.Time // when this node may first request it (first-seen + jitter)
	lastTry time.Time // when this node last requested it (for back-off)
}

// requestMissingSynthetics implements receiver-pull-on-gap healing. When an
// inbound synthetic stream is stalled (Received > Delivered) it means the
// predecessor Delivered+1 was never received, so nothing can advance until it
// arrives. Rather than wait for the source to notice and re-push, the receiver
// requests the missing message from the source partition's sequencer service
// and resubmits it locally; the existing MessageIsReady cascade then drains the
// whole pending tail. The re-submission is delivered in a block and advances
// Delivered on every validator at once, so exactly one successful request heals
// the entire partition — jitter + check-then-fire keep N validators from all
// firing. See issue #4064.
func (c *Conductor) requestMissingSynthetics(ctx context.Context, batch *database.Batch) error {
	// Default off until validated in production.
	if !def(c.EnableSyntheticHealing, false) {
		return nil
	}
	if c.Sequencer == nil {
		return nil
	}

	// Load this node's inbound synthetic ledger. Each entry tracks a source
	// partition we have received synthetics from.
	var ledger *protocol.SyntheticLedger
	err := batch.Account(c.Url(protocol.Synthetic)).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load synthetic ledger: %w", err)
	}

	now := time.Now()
	for _, part := range ledger.Sequence {
		// A gap exists only if we have received past what we have delivered.
		// The missing message is always Delivered+1 (receiving a later message
		// proves the source produced everything up to it).
		if part.Received <= part.Delivered {
			continue
		}
		want := part.Delivered + 1

		// Jittered check-then-fire: only one (or two) validators actually pull
		// before the gap closes for everyone.
		if !c.claimSyntheticRequest(part.Url, want, now) {
			continue
		}

		err := c.requestSyntheticFrom(ctx, part.Url, want)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to request missing synthetic",
				"source", part.Url, "destination", c.Url(), "number", want, "error", err)
			continue
		}
	}
	return nil
}

// claimSyntheticRequest returns true if this node should request source→self
// message `want` right now. It schedules a per-node random delay on first sight
// of a gap and enforces a back-off between requests, so a stalled stream isn't
// hammered by every validator on every block.
func (c *Conductor) claimSyntheticRequest(source *url.URL, want uint64, now time.Time) bool {
	window := c.SyntheticHealWindow
	if window <= 0 {
		window = syntheticHealWindow
	}

	c.synthHealMu.Lock()
	defer c.synthHealMu.Unlock()

	if c.synthHealState == nil {
		c.synthHealState = map[string]*synthHealEntry{}
	}

	key := source.String()
	e := c.synthHealState[key]
	if e == nil || e.want != want {
		// A new (or advanced) gap. Schedule this node's jittered fire time; do
		// not fire yet. Different validators pick independent delays, so they
		// wake at different times and the first re-submission closes the gap
		// for the rest.
		c.synthHealState[key] = &synthHealEntry{
			want:   want,
			fireAt: now.Add(randDuration(window)),
		}
		return false
	}

	// Check-then-fire: the gap is re-read from the ledger every block, so if
	// another validator already healed it, `want` will have advanced (or the
	// stream will no longer be stalled) and we never reach here.
	if now.Before(e.fireAt) {
		return false // still waiting out the jitter window
	}
	if !e.lastTry.IsZero() && now.Sub(e.lastTry) < window {
		return false // backing off since the last request
	}

	e.lastTry = now
	return true
}

// requestSyntheticFrom pulls a single missing synthetic message from the source
// partition's sequencer service and resubmits it into this partition.
func (c *Conductor) requestSyntheticFrom(ctx context.Context, source *url.URL, num uint64) error {
	// Bound the pull: a hung RPC must not pin the goroutine (and its read
	// batch) forever. The heal window is a natural bound — by the time it
	// expires the back-off would allow a fresh attempt anyway (#4066).
	window := c.SyntheticHealWindow
	if window <= 0 {
		window = syntheticHealWindow
	}
	ctx, cancel := context.WithTimeout(ctx, window)
	defer cancel()

	// Pull the missing synthetic — message, proof, and the source's signature —
	// from the source partition's sequencer. Routing sends this to whatever
	// live node serves the source partition; any of them holds the message
	// (the source provably produced it), so we don't target a specific node.
	r, err := c.Sequencer.Sequence(ctx, source.JoinPath(protocol.Synthetic), c.Url(), num, private.SequenceOptions{})
	if err != nil {
		return errors.UnknownError.WithFormat("request synthetic %v→%v #%d: %w", source, c.Url(), num, err)
	}

	env, err := c.buildSyntheticSubmission(r)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if env == nil {
		// Not yet provable (the source can't anchor it yet). A later scan will
		// retry once anchors catch up.
		return nil
	}

	// If the synthetic message is for a transaction (SignatureRequest,
	// CreditPayment), bundle the transaction itself, exactly like the normal
	// outbound path (block_begin.go) and the heal tool do — in a wedged stream
	// the destination may never have received it, and without it the healed
	// message fails on "load transaction" and the stream stays stuck (#4066).
	if m, ok := r.Sequence.Message.(messaging.MessageForTransaction); ok &&
		r.Sequence.Message.Type() != messaging.MessageTypeBlockAnchor {
		txr, err := c.Querier.QueryTransaction(ctx, source.WithTxID(m.GetTxID().Hash()), nil)
		if err != nil {
			return errors.UnknownError.WithFormat("query companion transaction for %v→%v #%d: %w", source, c.Url(), num, err)
		}
		env.Messages = append(env.Messages, txr.Message)
	}

	err = c.submit(ctx, c.Url(), env)
	if err != nil {
		return errors.UnknownError.WithFormat("submit synthetic %v→%v #%d: %w", source, c.Url(), num, err)
	}

	c.synthHeals.Add(1)
	if c.Heals != nil {
		c.Heals.Synthetic.Add(1)
	}
	mHeals.WithLabelValues("synthetic", c.Partition.ID, partitionLabel(source)).Inc()
	slog.InfoContext(ctx, "Requested missing synthetic transaction",
		"source", source, "destination", c.Url(), "number", num, "id", r.ID)
	return nil
}

// buildSyntheticSubmission assembles a submittable envelope from a sequencer
// response. The sequencer returns the same components the normal outbound path
// produces (block_begin.go): the sequenced message, an anchor proof, and the
// source's key signature — so no receipt needs to be rebuilt here. Returns a
// nil envelope (no error) if the message cannot yet be proven.
func (c *Conductor) buildSyntheticSubmission(r *api.MessageRecord[messaging.Message]) (*messaging.Envelope, error) {
	if r.Sequence == nil {
		return nil, errors.UnknownError.With("sequencer returned no sequenced message")
	}
	if r.SourceReceipt == nil {
		// The source cannot prove this message yet (its anchor is not
		// available). Skip; a later scan retries.
		return nil, nil
	}

	// Extract the source's key signature from the record.
	var keySig protocol.KeySignature
	if r.Signatures != nil {
		for _, set := range r.Signatures.Records {
			if set.Signatures == nil {
				continue
			}
			for _, s := range set.Signatures.Records {
				sm, ok := s.Message.(*messaging.SignatureMessage)
				if !ok {
					continue
				}
				if ks, ok := sm.Signature.(protocol.KeySignature); ok {
					keySig = ks
				}
			}
		}
	}
	if keySig == nil {
		return nil, errors.UnknownError.With("sequencer response is not signed")
	}

	proof := &protocol.AnnotatedReceipt{
		Receipt: r.SourceReceipt,
		Anchor:  &protocol.AnchorMetadata{Account: protocol.DnUrl()},
	}

	var inner messaging.Message
	if c.Globals.Load().ExecutorVersion.V2BaikonurEnabled() {
		inner = &messaging.SyntheticMessage{
			Message:   r.Sequence,
			Proof:     proof,
			Signature: keySig,
		}
	} else {
		inner = &messaging.BadSyntheticMessage{
			Message:   r.Sequence,
			Proof:     proof,
			Signature: keySig,
		}
	}

	// Guard against submitting a message with a bad signature; the destination
	// would reject it anyway.
	if !keySig.Verify(nil, r.Sequence) {
		return nil, errors.UnknownError.With("sequencer response signature is not valid")
	}

	return &messaging.Envelope{Messages: []messaging.Message{inner}}, nil
}

// partitionLabel returns the partition ID for a partition URL, for use as a
// metric label.
func partitionLabel(u *url.URL) string {
	if id, ok := protocol.ParsePartitionUrl(u); ok {
		return id
	}
	return u.ShortString()
}

// randDuration returns a uniform random duration in [0, max) drawn from
// crypto/rand, so validators independently stagger their heal requests.
func randDuration(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0
	}
	n := binary.BigEndian.Uint64(b[:]) % uint64(max)
	return time.Duration(n)
}

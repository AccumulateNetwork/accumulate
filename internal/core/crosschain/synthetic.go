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
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// syntheticHealWindow is the retry back-off, per-sequence suppression interval,
// and pull deadline for receiver-pull synthetic healing — how long a stalled
// stream waits between attempts and how long one pull may block. It is NOT the
// jitter; see syntheticHealJitter. See issue #4064.
const syntheticHealWindow = 10 * time.Second

// syntheticHealJitter is the maximum random delay a validator waits before its
// FIRST pull of a stalled stream, so N validators do not all fire at once. It is
// kept to ~1 block so healing starts promptly rather than up to ~8 blocks later
// (the old 10s "~10 block times" delay). Overlap when several validators fire
// within a block is harmless: pulls are idempotent and per-sequence suppression
// bounds the duplicate traffic.
const syntheticHealJitter = 1 * time.Second

// syntheticHealBatch caps how many missing messages one scan pulls per stream
// WHILE the source is keeping up. Draining a run of drops fast is only safe as
// long as the source's sequencer can answer; a batch of this size from every
// receiver at once is exactly what starves a busy source (#4067). Once the
// wedge tx stops clearing, healing drops to a batch of one — see wedgeFocusAfter.
const syntheticHealBatch = 200

// wedgeFocusAfter is how many batch attempts on the SAME wedge tx (Delivered+1)
// may fail before a stream switches from batch healing to focus healing. When a
// single gap is holding up the whole received tail and the source cannot keep
// up with the backlog, spraying it with 200 pulls per receiver keeps it starved.
// Delivering Delivered+1 alone unblocks everything behind it, so once the batch
// has failed to clear it, every request goes to that one sequence instead.
const wedgeFocusAfter = 1

// tailProbeInterval is how often a caught-up stream (Received == Delivered) is
// speculatively probed for a dropped TAIL message — one the receiver cannot see
// as a gap because Received never advanced past it. Kept several times the heal
// window because tail drops are rare and a quiet stream tolerates the reveal
// latency, so busy streams are not hammered with probe traffic. See #4070.
const tailProbeInterval = 3 * syntheticHealWindow

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

// mHealDeferred counts heal pulls that succeeded but could not be submitted
// because the message was not yet provable — the source has not anchored it far
// enough for a receipt. This is the silent path (requestSyntheticFrom returns
// nil with no error); without a counter a stream can wait on it forever unseen.
var mHealDeferred = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "accumulate", Subsystem: "crosschain", Name: "heal_deferred_total",
	Help: "Heal pulls deferred because the message is not yet provable (awaiting the covering anchor)",
}, []string{"type", "partition", "remote"})

// mHealErrors counts heal pulls that failed outright — source unreachable, no
// live peers, or the request deadline expired.
var mHealErrors = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "accumulate", Subsystem: "crosschain", Name: "heal_errors_total",
	Help: "Heal pulls that failed (unreachable source, no peers, deadline exceeded)",
}, []string{"type", "partition", "remote"})

// mHealFocus counts how many times a stalled stream dropped from batch healing
// to focus healing (pull only the wedge tx). A rising count means a source is
// being starved and healing is protecting it by narrowing to the one gap.
var mHealFocus = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "accumulate", Subsystem: "crosschain", Name: "heal_focus_total",
	Help: "Times a stalled stream switched to focus healing (single-wedge pull)",
}, []string{"type", "partition", "remote"})

// mHealStuck is how many times a stream's current wedge tx (Delivered+1) has been
// re-attempted without `delivered` advancing. mHeals counts SUBMITS, not
// deliveries, so a stream can churn — pull and re-submit the blocker forever
// while it never lands — with mHeals climbing and nothing else showing it. This
// gauge makes that churn visible: a high, non-decreasing value is a heal that is
// "succeeding" on submit but not delivering.
var mHealStuck = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "accumulate", Subsystem: "crosschain", Name: "heal_stuck_tries",
	Help: "Consecutive heal attempts on the same wedge tx without delivered advancing",
}, []string{"type", "partition", "remote"})

// mSequence exposes the live per-pair ledger state so a wedge is a gauge, not a
// log scrape. Labels are the actual stream direction — src and dst — so every
// series is unambiguous regardless of which side's ledger it came from: field
// "produced" is set by src, "received"/"delivered" by dst, and they merge on the
// same (src,dst) key. delivered lagging received is a wedge on src->dst;
// received lagging produced is in transit or lost.
var mSequence = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "accumulate", Subsystem: "crosschain", Name: "sequence",
	Help: "Max cross-partition sequence number for src->dst (field=produced|received|delivered)",
}, []string{"type", "field", "src", "dst"})

// synthHealEntry tracks this node's back-off state for a single stalled inbound
// synthetic stream.
type synthHealEntry struct {
	want    uint64    // the sequence number this stream is stalled on
	fireAt  time.Time // when this node may first request it (first-seen + jitter)
	lastTry time.Time // when this node last requested it (for back-off)
	tries   int       // fires on this same wedge tx without it clearing; resets when want advances
}

// claimSequence reports whether this node should (re)submit a heal for a
// specific sequence number. Suppression by Delivered alone is not enough: if
// delivery is blocked for any reason, Delivered never advances, every
// validator's back-off expires and they all re-submit the same holes forever —
// a heal storm that sustains the freeze (#4067). Bounding by DISTINCT hole
// instead of by elapsed time keeps heal traffic proportional to the real gap.
func (c *Conductor) claimSequence(source *url.URL, seq uint64, now time.Time, window time.Duration) bool {
	c.synthHealMu.Lock()
	defer c.synthHealMu.Unlock()
	if c.seqHealAt == nil {
		c.seqHealAt = map[string]time.Time{}
	}
	key := source.String() + "#" + strconv.FormatUint(seq, 10)
	if last, ok := c.seqHealAt[key]; ok && now.Sub(last) < window {
		return false
	}
	c.seqHealAt[key] = now

	// Bound the map: drop entries older than 10 windows.
	if len(c.seqHealAt) > 10000 {
		for k, t := range c.seqHealAt {
			if now.Sub(t) > 10*window {
				delete(c.seqHealAt, k)
			}
		}
	}
	return true
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
func (c *Conductor) requestMissingSynthetics(ctx context.Context) error {
	// Default off until validated in production.
	if !def(c.EnableSyntheticHealing, false) {
		return nil
	}
	if c.Sequencer == nil {
		return nil
	}

	// Read the inbound synthetic ledger under a short-lived batch and RELEASE it
	// before any network pull. The pulls below make P2P RPCs to source
	// sequencers; holding a database read batch open across a slow or hung peer
	// was the reason the pull had to be force-timed-out at the heal window,
	// which turned a slow BVN sequencer into a failed heal and a permanent wedge
	// (#4067). Nothing after this read needs the batch.
	batch := c.Database.Begin(false)
	var ledger *protocol.SyntheticLedger
	err := batch.Account(c.Url(protocol.Synthetic)).Main().GetAs(&ledger)
	batch.Discard()
	if err != nil {
		return errors.UnknownError.WithFormat("load synthetic ledger: %w", err)
	}

	now := time.Now()
	window := c.SyntheticHealWindow
	if window <= 0 {
		window = syntheticHealWindow
	}
	for _, part := range ledger.Sequence {
		// Publish this pair's live state as gauges so the flow matrix is read,
		// not scraped. This is the receiver's ledger: Produced is our outbound
		// (me->remote), Received/Delivered are our inbound (remote->me), so each
		// is labelled with its true direction.
		remote := partitionLabel(part.Url)
		me := c.Partition.ID
		mSequence.WithLabelValues("synthetic", "produced", me, remote).Set(float64(part.Produced))
		mSequence.WithLabelValues("synthetic", "received", remote, me).Set(float64(part.Received))
		mSequence.WithLabelValues("synthetic", "delivered", remote, me).Set(float64(part.Delivered))

		// A gap the receiver can SEE requires Received > Delivered. But a synthetic
		// dropped at the TAIL — the newest message, with nothing received after it —
		// never advances Received, so it leaves no Pending hole and is invisible to
		// the gap healers. It stays hidden until the source sends the next message
		// on this exact path, which on a quiet stream can be far off (or never).
		//
		// Speculatively pull Delivered+1: if the source produced it (a dropped or
		// in-transit tail) this reveals and heals it, and the next scan drains the
		// one after it; NotFound means the source has produced nothing more and the
		// stream is genuinely caught up (#4070). Throttled to one probe per stream
		// per tailProbeInterval — keyed with sentinel sequence 0, which never
		// collides with a real heal — so busy streams are not hammered.
		if part.Received <= part.Delivered {
			if c.claimSequence(part.Url, 0, now, tailProbeInterval) {
				probe := part.Delivered + 1
				if err := c.requestSyntheticFrom(ctx, part.Url, probe); err != nil && !errors.Is(err, errors.NotFound) {
					slog.ErrorContext(ctx, "Failed to probe synthetic tail",
						"source", part.Url, "destination", c.Url(), "number", probe, "error", err)
				}
			}
			continue
		}
		want := part.Delivered + 1

		// Jittered check-then-fire: only one (or two) validators actually pull
		// before the gap closes for everyone. `focus` is set once the batch has
		// failed to clear this wedge tx — the source is starved. `tries` is how
		// long this same wedge has failed to clear: the churn signal.
		fire, focus, tries := c.claimSyntheticRequest(part.Url, want, now)
		mHealStuck.WithLabelValues("synthetic", c.Partition.ID, remote).Set(float64(tries))
		// A wedge that keeps being re-attempted without delivered moving is the
		// "submit succeeds, delivery frozen" case mHeals hides. Surface it — with
		// the exact blocking sequence — so it can be traced through execution.
		if tries >= wedgeFocusAfter+1 && tries%8 == wedgeFocusAfter+1 {
			slog.WarnContext(ctx, "Synthetic wedge not clearing",
				"source", part.Url, "destination", c.Url(), "wedge", want,
				"tries", tries, "received", part.Received, "delivered", part.Delivered)
		}
		if !fire {
			continue
		}
		if focus {
			mHealFocus.WithLabelValues("synthetic", c.Partition.ID, remote).Inc()
		}

		// Focus mode: one gap (Delivered+1) is holding up the whole received
		// tail and the source cannot keep up. Request ONLY that sequence.
		// Delivering it advances Delivered and the MessageIsReady cascade drains
		// everything received behind it; the next hole (if any) becomes the new
		// priority next scan. This collapses the load on a starving source from
		// ~batch-per-receiver to one-per-receiver, so it can actually answer.
		if focus {
			if c.claimSequence(part.Url, want, now, window) {
				if err := c.requestSyntheticFrom(ctx, part.Url, want); err != nil {
					slog.ErrorContext(ctx, "Failed to request wedge synthetic",
						"source", part.Url, "destination", c.Url(), "number", want, "error", err)
				}
			}
			continue
		}

		// Source keeping up: pull EVERY missing message in the pending window
		// (nil entries), not just Delivered+1 — a run of consecutive drops would
		// otherwise heal one message per window. Capped per scan; each pull is
		// idempotent.
		healed := 0
		for i, txid := range part.Pending {
			if txid != nil {
				continue
			}
			seq := part.Delivered + 1 + uint64(i)
			if !c.claimSequence(part.Url, seq, now, window) {
				continue
			}
			err := c.requestSyntheticFrom(ctx, part.Url, seq)
			if err != nil {
				// Never abandon the batch: delivery is ordered, so failing to
				// pull the FIRST hole must not stop us pulling the rest — and
				// a stream must never wedge because one pull failed (#4067).
				slog.ErrorContext(ctx, "Failed to request missing synthetic",
					"source", part.Url, "destination", c.Url(), "number", seq, "error", err)
				continue
			}
			healed++
			if healed >= syntheticHealBatch {
				break
			}
		}
		if healed == 0 {
			// Delivered+1 may be missing without a pending entry
			err := c.requestSyntheticFrom(ctx, part.Url, want)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to request missing synthetic",
					"source", part.Url, "destination", c.Url(), "number", want, "error", err)
				continue
			}
		}
	}
	return nil
}

// claimSyntheticRequest returns true if this node should request source→self
// message `want` right now. It schedules a per-node random delay on first sight
// of a gap and enforces a back-off between requests, so a stalled stream isn't
// hammered by every validator on every block.
// claimSyntheticRequest returns whether to fire, whether to focus on the single
// wedge tx, and `tries` — how many times this same wedge has already been
// attempted without clearing (0 for a fresh gap). A rising `tries` is the churn
// signal: the healer keeps re-submitting a blocker that never lands.
func (c *Conductor) claimSyntheticRequest(source *url.URL, want uint64, now time.Time) (fire, focus bool, tries int) {
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
		// A new (or advanced) gap — including the wedge tx having just cleared,
		// which advances `want`. Reset to batch mode (tries = 0): progress means
		// the source is keeping up. Schedule this node's jittered fire time; do
		// not fire yet. Different validators pick independent delays, so they
		// wake at different times and the first re-submission closes the gap
		// for the rest.
		jitter := syntheticHealJitter
		if jitter > window {
			jitter = window // keep the first delay within the back-off window
		}
		c.synthHealState[key] = &synthHealEntry{
			want:   want,
			fireAt: now.Add(randDuration(jitter)),
		}
		return false, false, 0
	}

	// Check-then-fire: the gap is re-read from the ledger every block, so if
	// another validator already healed it, `want` will have advanced (or the
	// stream will no longer be stalled) and we never reach here.
	if now.Before(e.fireAt) {
		return false, false, e.tries // still waiting out the jitter window
	}
	if !e.lastTry.IsZero() && now.Sub(e.lastTry) < window {
		return false, false, e.tries // backing off since the last request
	}

	// The same wedge tx is still blocking the stream. The first attempt pulls a
	// batch (catching up on a run of drops is fine while the source keeps up);
	// once that has failed to clear `want`, focus every request on Delivered+1
	// alone rather than re-spraying the backlog and keeping the source starved.
	focus = e.tries >= wedgeFocusAfter
	tries = e.tries
	e.lastTry = now
	e.tries++
	return true, focus, tries
}

// requestSyntheticFrom pulls a single missing synthetic message from the source
// partition's sequencer service and resubmits it into this partition.
func (c *Conductor) requestSyntheticFrom(ctx context.Context, source *url.URL, num uint64) error {
	window := c.SyntheticHealWindow
	if window <= 0 {
		window = syntheticHealWindow
	}
	miss := func() { mHealErrors.WithLabelValues("synthetic", c.Partition.ID, partitionLabel(source)).Inc() }

	// Pull the missing synthetic — message, proof, and the source's signature —
	// from the source partition's sequencer. Routing sends this to whatever live
	// node serves the source partition; any of them holds the message, so we
	// don't target a specific node.
	//
	// The deadline is PER ATTEMPT, not one shared budget. A slow or hung peer
	// times out on its own attempt and routing lands the retry on a different
	// node — instead of one hung peer consuming the whole budget and failing the
	// heal, which is exactly how a slow BVN2 sequencer wedged the soak (#4067).
	// Safe to be generous now that the pull holds no database batch.
	var r *api.MessageRecord[messaging.Message]
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		actx, cancel := context.WithTimeout(ctx, window)
		r, err = c.Sequencer.Sequence(actx, source.JoinPath(protocol.Synthetic), c.Url(), num, private.SequenceOptions{})
		cancel()
		if err == nil {
			break
		}
		if errors.Is(err, errors.NotFound) {
			break // the source has not produced this number — caught up, don't retry
		}
		if ctx.Err() != nil {
			break // parent cancelled; stop
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		// NotFound means the source has not produced `num` yet: a tail probe found
		// the stream caught up. That is not a failure, so return it unwrapped and
		// do not count it as a heal error.
		if errors.Is(err, errors.NotFound) {
			return err
		}
		miss()
		return errors.UnknownError.WithFormat("request synthetic %v→%v #%d: %w", source, c.Url(), num, err)
	}

	env, err := c.buildSyntheticSubmission(r)
	if err != nil {
		miss()
		return errors.UnknownError.Wrap(err)
	}
	if env == nil {
		// Not yet provable (the source can't anchor it yet). A later scan will
		// retry once anchors catch up — but count it, because a stream stuck here
		// is otherwise silent (no error, no delivery).
		mHealDeferred.WithLabelValues("synthetic", c.Partition.ID, partitionLabel(source)).Inc()
		return nil
	}

	// If the synthetic message is for a transaction (SignatureRequest,
	// CreditPayment), bundle the transaction itself, exactly like the normal
	// outbound path (block_begin.go) and the heal tool do — in a wedged stream
	// the destination may never have received it, and without it the healed
	// message fails on "load transaction" and the stream stays stuck (#4066).
	if m, ok := r.Sequence.Message.(messaging.MessageForTransaction); ok &&
		r.Sequence.Message.Type() != messaging.MessageTypeBlockAnchor {
		qctx, cancel := context.WithTimeout(ctx, window)
		txr, err := c.Querier.QueryTransaction(qctx, source.WithTxID(m.GetTxID().Hash()), nil)
		cancel()
		if err != nil {
			miss()
			return errors.UnknownError.WithFormat("query companion transaction for %v→%v #%d: %w", source, c.Url(), num, err)
		}
		env.Messages = append(env.Messages, txr.Message)
	}

	sctx, cancel := context.WithTimeout(ctx, window)
	err = c.submit(sctx, c.Url(), env)
	cancel()
	if err != nil {
		miss()
		return errors.UnknownError.WithFormat("submit synthetic %v→%v #%d: %w", source, c.Url(), num, err)
	}

	c.synthHeals.Add(1)
	if c.Heals != nil {
		c.Heals.Synthetic.Add(1)
	}
	mHeals.WithLabelValues("synthetic", c.Partition.ID, partitionLabel(source)).Inc()
	// WARN, not INFO: this is the record of what the healer actually pulled and
	// re-submitted. When a stream is churning we need to SEE the blocker sequence
	// being pulled (id lets us then follow it through execution), and INFO is
	// filtered on the soak nodes.
	slog.WarnContext(ctx, "Re-submitted missing synthetic",
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

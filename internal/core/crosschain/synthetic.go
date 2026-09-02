// Copyright 2026 The Accumulate Authors
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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
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

// syntheticHealBatch caps how many missing messages one scan pulls per stream.
// Drain time must not scale with backlog: a stream that missed hundreds of
// messages during an outage has to catch up in a window or two, not minutes
// (#4067). Safe to be large because per-sequence suppression bounds heal
// traffic by DISTINCT hole; the remaining bound is mempool capacity.
const syntheticHealBatch = 200

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

// mReconcile counts interval-reconcile pull ATTEMPTS and successes separately.
// Counting only successes makes a reconcile that fails every time
// indistinguishable from one with nothing to do — both report zero, which hid a
// completely broken pull path across six soak runs (#4073).
var mReconcile = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "crosschain",
	Name:      "reconcile_pulls_total",
	Help:      "Interval-reconcile pull attempts by outcome",
}, []string{"outcome"}) // attempted | succeeded | failed

// synthHealEntry tracks this node's back-off state for a single stalled inbound
// synthetic stream.
type synthHealEntry struct {
	want    uint64    // the sequence number this stream is stalled on
	fireAt  time.Time // when this node may first request it (first-seen + jitter)
	lastTry time.Time // when this node last requested it (for back-off)
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
// inbound synthetic stream is stalled — the node is holding something it cannot
// execute — the predecessor Delivered+1 was never received, so nothing can
// advance until it arrives. Rather than wait for the source to notice and re-push, the receiver
// requests the missing message from the source partition's sequencer service
// and resubmits it locally; the existing MessageIsReady cascade then drains the
// whole pending tail. The re-submission is delivered in a block and advances
// Delivered on every validator at once, so exactly one successful request heals
// the entire partition — jitter + check-then-fire keep N validators from all
// firing. See issue #4064.
func (c *Conductor) requestMissingSynthetics(ctx context.Context, batch *database.Batch) error {
	if c.Sequencer == nil {
		return nil
	}

	// The ledger is opened for one thing only — Delivered, what has been
	// processed. Nothing else about a stream lives there any more (#4189), and
	// a message already processed needs nothing done about it.
	var ledger *protocol.SyntheticLedger
	err := batch.Account(c.Url(protocol.Synthetic)).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load synthetic ledger: %w", err)
	}

	now := time.Now()
	window := c.SyntheticHealWindow
	if window <= 0 {
		window = syntheticHealWindow
	}
	for _, source := range c.inboundSources(ledger) {
		// Partition returns the entry if there is one and a zero entry if there
		// is not. A stream that has delivered nothing is delivered through 0,
		// which is the right answer and not a missing one.
		part := ledger.Partition(source)

		// A gap exists only if we are holding something past what we have
		// delivered. The missing message is always Delivered+1 (receiving a
		// later message proves the source produced everything up to it).
		if c.sighted(batch, part) <= part.Delivered {
			continue
		}
		// Circuit breaker: a source that failed the last several pulls is down
		// or drowning; pulling harder starves the transport for everyone
		// (#4115). Skip it until its backoff expires.
		if !c.remoteAllowed(part.Url.String()) {
			continue
		}
		want := part.Delivered + 1

		// Jittered check-then-fire: only one (or two) validators actually pull
		// before the gap closes for everyone.
		if !c.claimSyntheticRequest(part.Url, want, now) {
			continue
		}

		// Range requests under collection proofs beat N per-message pulls,
		// and unlike them they do not need the directory to have caught up
		// (#4087). Every missing run is pulled under the SAME held anchor —
		// it postdates the newest hole, so it proves all of them — oldest
		// first, so in-order delivery unblocks as each run lands. Falls
		// through to the per-message path when unavailable or when any run
		// could not be pulled — a stream must never wedge because the fast
		// path was not usable.
		if held, ok := c.rangeProofAnchor(batch, part.Url); ok {
			if runs := c.missingRuns(batch, part); len(runs) > 0 {
				budget := uint64(syntheticHealBatch)
				allDone := true
				for _, r := range runs {
					if budget == 0 {
						allDone = false
						break
					}
					first, last := r[0], r[1]
					if last-first+1 > budget {
						last = first + budget - 1
						allDone = false
					}
					done, err := c.recoverSyntheticsViaRange(ctx, part.Url, first, last, held)
					switch {
					case err != nil:
						allDone = false
						c.classifyRemoteError(part.Url.String(), err)
						slog.ErrorContext(ctx, "Failed to recover synthetics by range",
							"source", part.Url, "destination", c.Url(), "start", first, "end", last, "error", err)
					case done:
						c.remoteOK(part.Url.String())
						budget -= last - first + 1
					default:
						allDone = false
					}
					// A source whose circuit just opened is down or drowning;
					// stop hammering it with the remaining runs.
					if !c.remoteAllowed(part.Url.String()) {
						allDone = false
						break
					}
				}
				if allDone {
					continue
				}
			}
		}

		// Pull EVERY missing message, not just Delivered+1 — a run of
		// consecutive drops or a dense drop pattern would otherwise heal one
		// message per window and never catch up (#4067). Staging knows exactly
		// which sequence numbers are holes; batch them, capped per scan. Each
		// pull is idempotent.
		healed := 0
		for _, seq := range c.missingNumbers(batch, part) {
			if !c.claimSequence(part.Url, seq, now, window) {
				continue
			}
			err := c.requestSyntheticFrom(ctx, part.Url, seq)
			if err != nil {
				// Never abandon the batch: delivery is ordered, so failing to
				// pull the FIRST hole must not stop us pulling the rest — and
				// a stream must never wedge because one pull failed (#4067).
				// But feed the breaker: enough consecutive failures and the
				// source is skipped for a backoff instead of hammered.
				c.classifyRemoteError(part.Url.String(), err)
				slog.ErrorContext(ctx, "Failed to request missing synthetic",
					"source", part.Url, "destination", c.Url(), "number", seq, "error", err)
				if !c.remoteAllowed(part.Url.String()) {
					break
				}
				continue
			}
			c.remoteOK(part.Url.String())
			healed++
			if healed >= syntheticHealBatch {
				break
			}
		}
		if healed == 0 && c.remoteAllowed(part.Url.String()) {
			// Delivered+1 may be missing without a pending entry
			err := c.requestSyntheticFrom(ctx, part.Url, want)
			if err != nil {
				c.classifyRemoteError(part.Url.String(), err)
				slog.ErrorContext(ctx, "Failed to request missing synthetic",
					"source", part.Url, "destination", c.Url(), "number", want, "error", err)
				continue
			}
			c.remoteOK(part.Url.String())
		}
	}
	return nil
}

// rangeProofAnchor returns the anchor a range request should be proven against:
// the newest one from this source that we have EXECUTED, so its root is on our
// anchor chain for that partition and the destination-side check will find it.
//
// Separate from the pull itself because callers must know whether the range path
// is usable BEFORE they claim a sequence. Claiming and then falling back would
// leave the per-message path suppressed by the claim the fast path just made.
//
// Not usable when nothing from the source has ever been anchored into us: there
// is then no root to verify against. That is the case for BVN->BVN streams,
// where partitions hold no anchors from each other and the chain of trust runs
// through the directory by construction.
func (c *Conductor) rangeProofAnchor(batch *database.Batch, source *url.URL) (uint64, bool) {
	if !c.Globals.Load().ExecutorVersion.V2KourouEnabled() {
		return 0, false
	}
	if _, ok := c.Sequencer.(private.SequenceRanger); !ok {
		return 0, false
	}

	var anchors *protocol.AnchorLedger
	err := batch.Account(c.Url(protocol.AnchorPool)).Main().GetAs(&anchors)
	if err != nil {
		slog.Debug("Cannot load anchor ledger for range proof", "module", "synthetic", "error", err)
		return 0, false
	}
	held := anchors.Partition(source).Delivered
	return held, held > 0
}

// missingRuns returns every contiguous run of numbers above the delivery point
// that staging does not hold, oldest first.
//
// It asks STAGING, not the ledger (#4189). It used to walk
// PartitionSyntheticLedger.Pending for nil entries, which is what the block
// wrote — and past that array's bound the executor stored a message while
// refusing to record it, so the ledger reported holes for messages already in
// the local database and this function sent the network to fetch them back. Each run is what one range request covers: a
// collection proof is a merkle range, so it proves a run of adjacent entries,
// not an arbitrary selection. But ONE held anchor proves ALL of them — the
// anchor postdates the newest hole (holes are by definition below Received,
// and anchors keep arriving), and an anchor proves everything produced before
// it by replay. So a scan can pull every run under the same anchor in one
// pass; there is no reason to heal one run per window. Oldest first because
// delivery is in-order: the stream advances the moment the oldest run fills,
// and keeps advancing as each next run lands.
//
// Observed need: the 20260824T024122Z wedge left 63 holes scattered across
// ~20 runs (498-506, 514, 518, 539-542, ...). One run per heal window would
// have taken ~20 windows to converge; one scan covers them all.
func (c *Conductor) missingRuns(batch *database.Batch, part *protocol.PartitionSyntheticLedger) [][2]uint64 {
	id := c.syntheticStream(part.Url)
	runs, err := execute.Missing(batch, id, part.Delivered, c.sighted(batch, part), syntheticHealBatch)
	if err != nil {
		slog.Error("Failed to read staging", "module", "synthetic", "source", part.Url, "error", err)
		return nil
	}
	return runs
}

// firstMissingRun returns the first contiguous run of holes above the delivery
// point.
func (c *Conductor) firstMissingRun(batch *database.Batch, part *protocol.PartitionSyntheticLedger) (uint64, uint64, bool) {
	runs := c.missingRuns(batch, part)
	if len(runs) == 0 {
		return 0, 0, false
	}
	return runs[0][0], runs[0][1], true
}

// missingNumbers flattens the runs into the individual numbers to pull, capped
// per scan. Flat rather than nested because every failure rule below acts on
// the whole scan — a breaker that opens stops the scan, not the run.
func (c *Conductor) missingNumbers(batch *database.Batch, part *protocol.PartitionSyntheticLedger) []uint64 {
	var nums []uint64
	for _, r := range c.missingRuns(batch, part) {
		for seq := r[0]; seq <= r[1] && len(nums) < syntheticHealBatch; seq++ {
			nums = append(nums, seq)
		}
		if len(nums) >= syntheticHealBatch {
			break
		}
	}
	return nums
}

// inboundSources lists the partitions this node may be receiving from.
//
// Primarily from the network definition, NOT from our own ledger: a stream that
// has only staged has delivered nothing, so it has no ledger entry — and that
// is exactly the stream most likely to be stuck. Before #4189 an entry existed
// regardless, because receipts were written into the ledger, which is the
// coupling being removed.
//
// The ledger's own entries are unioned in so a node whose globals have not
// arrived yet still scans the streams it has delivered from. Sorted, because
// every node deriving the same thing the same way is cheap insurance.
func (c *Conductor) inboundSources(ledger *protocol.SyntheticLedger) []*url.URL {
	seen := map[string]bool{}
	var sources []*url.URL
	add := func(u *url.URL) {
		if u == nil || strings.EqualFold(u.String(), c.Url().String()) || seen[u.String()] {
			return
		}
		seen[u.String()] = true
		sources = append(sources, u)
	}

	if g := c.Globals.Load(); g != nil && g.Network != nil {
		for _, peer := range g.Network.Partitions {
			if !strings.EqualFold(peer.ID, c.Partition.ID) {
				add(protocol.PartitionUrl(peer.ID))
			}
		}
	}
	for _, part := range ledger.Sequence {
		add(part.Url)
	}

	sort.Slice(sources, func(i, j int) bool { return sources[i].String() < sources[j].String() })
	return sources
}

// syntheticStream and anchorStream name the staging streams for messages
// inbound from a source. They are SEPARATE streams between the same pair of
// partitions — anchors are tracked by the anchor pool, synthetics by the
// synthetic account — so conflating them would let an anchor's position gate a
// synthetic's.
func (c *Conductor) syntheticStream(source *url.URL) execute.StreamID {
	return execute.StreamID{Ledger: c.Url(protocol.Synthetic), Source: source}
}

func (c *Conductor) anchorStream(source *url.URL) execute.StreamID {
	return execute.StreamID{Ledger: c.Url(protocol.AnchorPool), Source: source}
}

// sighted is the highest sequence number this node has seen from a source,
// whether it executed it or is still holding it.
//
// It replaces the ledger's Received, which was written by the block and is not
// written any more. The two halves come from the two places that own them: the
// ledger says what was delivered, staging says what is held, and the larger of
// the two is what the node has evidence of. Seeing a number is what proves the
// source produced everything below it, which is what makes a hole a hole rather
// than a stream that has simply not started.
func (c *Conductor) sighted(batch *database.Batch, part *protocol.PartitionSyntheticLedger) uint64 {
	return c.sightedOn(batch, c.syntheticStream(part.Url), part.Delivered)
}

// sightedOn is sighted for a stream named directly, which the anchor path needs
// because its entries come from the anchor ledger.
func (c *Conductor) sightedOn(batch *database.Batch, id execute.StreamID, delivered uint64) uint64 {
	h, err := execute.Sighted(batch, id)
	if err != nil {
		slog.Error("Failed to read staging", "module", "synthetic", "source", id.Source, "error", err)
		return delivered
	}
	if h > delivered {
		return h
	}
	return delivered
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
	// Retry: routing picks a peer per attempt and marks failures bad, so a
	// retry lands on a different node. Without this, ONE transient
	// "no live peers" permanently wedged a stream in the #4067 soak — the
	// mechanism meant to prevent wedges became the cause of one.
	var r *api.MessageRecord[messaging.Message]
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		r, err = c.Sequencer.Sequence(ctx, source.JoinPath(protocol.Synthetic), c.Url(), num, private.SequenceOptions{})
		if err == nil {
			break
		}
		// A NotFound ("reached the end of the chain") is a deterministic
		// answer about the source's state, not a transport hiccup — retrying
		// it milliseconds later tripled the pull storm for no possible gain
		// (#4086, #4115).
		if errors.Is(err, errors.NotFound) {
			break
		}
		select {
		case <-ctx.Done():
			return errors.UnknownError.WithFormat("request synthetic %v→%v #%d: %w", source, c.Url(), num, ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
	if err != nil {
		return errors.UnknownError.WithFormat("request synthetic %v→%v #%d: %w", source, c.Url(), num, err)
	}

	env, err := c.buildSyntheticSubmission(r, nil)
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
//
// A non-nil proof overrides the per-message one: that is the collection proof
// covering a whole range, which every record of that range shares.
func (c *Conductor) buildSyntheticSubmission(r *api.MessageRecord[messaging.Message], proof *protocol.AnnotatedReceipt) (*messaging.Envelope, error) {
	if r.Sequence == nil {
		return nil, errors.UnknownError.With("sequencer returned no sequenced message")
	}
	if proof == nil && r.SourceReceipt == nil {
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

	if proof == nil {
		proof = &protocol.AnnotatedReceipt{
			Receipt: r.SourceReceipt,
			Anchor:  &protocol.AnchorMetadata{Account: protocol.DnUrl()},
		}
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
	// would reject it anyway. This applies only to per-message receipts: with
	// a collection proof the proof is the authorization — the destination
	// accepts the message on hash inclusion alone, and the signature rides
	// along for identity. Refusing to submit a proven message over its
	// signature would wedge recovery after validator churn.
	if proof.ReceiptList == nil && !keySig.Verify(nil, r.Sequence) {
		return nil, errors.UnknownError.With("sequencer response signature is not valid")
	}

	return &messaging.Envelope{Messages: []messaging.Message{inner}}, nil
}

// recoverSyntheticsViaRange pulls the run [first, last] from source with ONE
// range request carrying ONE collection proof covering every message in it, and
// submits them (#4087).
//
// The proof is rooted at an anchor we ALREADY HOLD from that source, and we name
// which one in the request. That is the whole point, and it is what the
// per-message path cannot do: the individual receipt the source builds is
// continued to a directory-anchored root, so a message produced in a block the
// directory has not caught up to cannot be served at all — 1,492 such failures
// in ten minutes in #4086, and no constant-sized grace period can bound a lag
// that is unbounded.
//
// Rooting at an anchor we hold removes the directory from the path entirely. An
// anchor from S commits to S's root chain, which commits to S's synthetic
// chains, so a validated anchor from S proves every synthetic S produced before
// it — by replay, against a root we already trust. The source is not asked to
// prove anything; it returns the messages and the merkle state to replay them
// onto. And because we name the anchor, we can never be handed a proof we are
// unable to check.
//
// held is the anchor to prove against, from rangeProofAnchor. Returns false when
// the range path is unavailable, so callers fall back to per-message pulls
// rather than leaving a stream stuck.
func (c *Conductor) recoverSyntheticsViaRange(ctx context.Context, source *url.URL, first, last, held uint64) (bool, error) {
	ranger, ok := c.Sequencer.(private.SequenceRanger)
	if !ok {
		return false, nil // Peer does not serve ranges
	}
	if first == 0 || last < first {
		return false, nil
	}
	if last-first+1 > protocol.MaxReceiptListElements {
		last = first + protocol.MaxReceiptListElements - 1
	}

	records, err := ranger.SequenceRange(ctx, source.JoinPath(protocol.Synthetic), c.Url(), first, last,
		private.SequenceOptions{ProveAgainstAnchor: held})
	if err != nil {
		return false, errors.UnknownError.WithFormat("request synthetic range [%d, %d] from %v: %w", first, last, source, err)
	}
	if len(records) == 0 {
		return false, nil
	}

	// One proof, carried on the last record, covers the whole run.
	list := records[len(records)-1].SourceReceiptList
	if list == nil {
		return false, errors.InvalidRecord.WithFormat("synthetic range response from %v carries no collection proof", source)
	}
	proof := &protocol.AnnotatedReceipt{
		ReceiptList: list,
		Anchor:      &protocol.AnchorMetadata{Account: source},
	}

	for _, r := range records {
		env, err := c.buildSyntheticSubmission(r, proof)
		if err != nil {
			return false, errors.UnknownError.Wrap(err)
		}
		if env == nil {
			continue
		}

		// Bundle the companion transaction, exactly as the per-message path does:
		// in a wedged stream the destination may never have received it, and
		// without it the healed message fails on "load transaction" (#4066).
		if m, ok := r.Sequence.Message.(messaging.MessageForTransaction); ok &&
			r.Sequence.Message.Type() != messaging.MessageTypeBlockAnchor {
			txr, err := c.Querier.QueryTransaction(ctx, source.WithTxID(m.GetTxID().Hash()), nil)
			if err != nil {
				return false, errors.UnknownError.WithFormat("query companion transaction for %v→%v #%d: %w", source, c.Url(), r.Sequence.Number, err)
			}
			env.Messages = append(env.Messages, txr.Message)
		}

		err = c.submit(ctx, c.Url(), env)
		if err != nil {
			return false, errors.UnknownError.WithFormat("submit synthetic %v→%v #%d: %w", source, c.Url(), r.Sequence.Number, err)
		}
		c.synthHeals.Add(1)
		if c.Heals != nil {
			c.Heals.Synthetic.Add(1)
		}
		mHeals.WithLabelValues("synthetic-range", c.Partition.ID, partitionLabel(source)).Inc()
	}

	// Warn for the same reason the reconcile logs at Warn: this only happens when
	// messages were lost, and it has to be visible at a node's default level or
	// the path cannot be told apart from one that never ran.
	slog.WarnContext(ctx, "Recovered synthetic messages by range", "module", "synthetic",
		"source", source, "destination", c.Url(), "start", first, "end", last, "under-anchor", held)
	return true, nil
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

// reconcileInterval is how often (in blocks) a partition asks its peers what
// they have produced for it. Every other recovery path is reactive — it needs a
// later message to expose the hole — so on a stream carrying a couple of
// messages a day nothing ever triggers them.
//
// Every block, so a stalled stream recovers in about a second at mainnet block
// times rather than waiting out an interval. The cost of checking is a read per
// peer that produces nothing unless something is actually missing: one query per
// second on mainnet's two partitions, three on a four-partition network. The
// pull itself is still rate-limited per (source, sequence) by
// claimSyntheticRequest, so detecting every block cannot turn into repeated
// requests for the same message.
const reconcileInterval = 1

// reconcileGraceBlocks is how long a gap must persist before the reconcile acts
// on it. Produced-at-source minus received-at-destination is routinely non-zero
// on a healthy network — a message counts as produced the moment the source
// emits it and has not reached us yet. Acting immediately means requesting
// messages that are merely in flight, and the source cannot even build a receipt
// for them because they are not anchored yet, so every such pull fails with
// "reached the end of the chain".
//
// It must also exceed ANCHORING latency, not just delivery latency. The source
// builds a receipt from its own anchor index chain, so a sequence produced in
// block N cannot be served until block N is anchored — asking sooner fails with
// "locate anchor index chain entry for block N: reached the end of the chain".
// At 10 blocks that was 79 of 102 attempts. 60 blocks is ~60s at mainnet block
// times, well past both delivery and anchoring, and costs nothing: a genuinely
// lost message is lost for good, so a minute of patience is free.
//
// Counted in BLOCKS, not wall clock: simulator blocks are not time-paced, so a
// duration-based grace is meaningless there and the bug would go untested.
const reconcileGraceBlocks = 60

// reconcileInboundStreams asks every peer partition what it has produced for
// this partition and pulls anything that was never received.
//
// This is the one recovery path that does not depend on a subsequent message
// existing. requestMissingSynthetics needs something sighted above Delivered; a
// stream that lost its FIRST messages has nothing — staging holds nothing, the
// watermark is 0, and it is indistinguishable from a healthy idle stream. A lost tail
// looks the same. Observed live: DN->BVN1 sat at produced=2 received=0 for 23
// hours while every gap-based check reported healthy (#4073).
//
// Counterparties come from the network definition rather than from our own
// ledger, so this works even when we hold no entry at all for a source — which
// is exactly the state a lost prefix leaves behind.
func (c *Conductor) reconcileInboundStreams(ctx context.Context, batch *database.Batch, blockIndex uint64) error {
	if c.Sequencer == nil {
		return nil
	}

	// What we have received from each source.
	var ledger *protocol.SyntheticLedger
	err := batch.Account(c.Url(protocol.Synthetic)).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load synthetic ledger: %w", err)
	}
	// The high-water mark per source: what was delivered, or what staging is
	// holding above it. This is the ledger's old Received, recomputed from the
	// two records that now own its two halves (#4189).
	received := map[[32]byte]uint64{}
	for _, part := range ledger.Sequence {
		if part.Url != nil {
			received[part.Url.AccountID32()] = c.sighted(batch, part)
		}
	}

	globals := c.Globals.Load()
	if globals == nil {
		return nil
	}

	me := c.Url()
	now := time.Now()
	window := c.SyntheticHealWindow
	if window <= 0 {
		window = syntheticHealWindow
	}
	for _, peer := range globals.Network.Partitions {
		if strings.EqualFold(peer.ID, c.Partition.ID) {
			continue
		}
		source := protocol.PartitionUrl(peer.ID)

		// Circuit breaker, same as every other pull path. The reconcile loop
		// was the one healer left unwired, and it alone produced 157k failed
		// pulls in the 17-minute 20260820T050616Z shakedown — ~13/s per node
		// of retries against sources that kept answering "end of the chain"
		// (#4086, #4115).
		if !c.remoteAllowed(source.String()) {
			continue
		}

		// Ask the source what it has produced for us. The answer is an unproven
		// hint and does not need to be trusted: acting on it means issuing a
		// pull, and the pulled message arrives proof-carrying and is verified
		// like any other synthetic. A lying peer can at worst make us request
		// messages that do not exist, which fails harmlessly.
		var remote *protocol.SyntheticLedger
		_, err := c.Querier.QueryAccountAs(ctx, source.JoinPath(protocol.Synthetic), nil, &remote)
		if err != nil {
			// An unreachable peer must never wedge a stream (#4067). Try again
			// next window.
			// Warn, not Debug: a reconcile that cannot reach its peer is the
			// mechanism silently not working, which is indistinguishable from
			// "nothing to do" unless it says so.
			slog.WarnContext(ctx, "Reconcile: cannot query source ledger",
				"module", "synthetic", "source", source, "error", err)
			continue
		}

		var produced uint64
		for _, part := range remote.Sequence {
			if part.Url != nil && part.Url.Equal(me) {
				produced = part.Produced
				break
			}
		}

		have := received[source.AccountID32()]
		c.pruneOverdue(source, have)
		if produced <= have {
			continue
		}

		// Prefer one range request under one collection proof (#4087). Only the
		// OVERDUE tail is pulled, starting at have+1 where have is the
		// HIGH-WATER mark of what has been sighted. Under DAG-BFT that mark
		// advances past interior holes (messages arrive out of order), so
		// have+1 is NOT necessarily the oldest missing sequence — the holes
		// below it are the numbers staging does not hold, and they belong to
		// requestMissingSynthetics, which asks for them directly. Reconcile's job
		// is only the tail the destination cannot see: sequences the source has
		// produced that have never been sighted here at all.
		//
		// Availability is checked BEFORE claiming: claiming and then falling back
		// would leave the per-message path suppressed by the claim the fast path
		// just made, and the stream would sit unhealed until the window expired.
		if held, ok := c.rangeProofAnchor(batch, source); ok {
			overdue := have
			for seq := have + 1; seq <= produced && overdue-have < syntheticHealBatch; seq++ {
				if !c.gapIsOverdue(source, seq, blockIndex) {
					break
				}
				overdue = seq
			}
			if overdue > have && c.claimSequence(source, have+1, now, window) {
				mReconcile.WithLabelValues("attempted").Inc()
				done, err := c.recoverSyntheticsViaRange(ctx, source, have+1, overdue, held)
				switch {
				case err != nil:
					mReconcile.WithLabelValues("failed").Inc()
					c.classifyRemoteError(source.String(), err)
					slog.ErrorContext(ctx, "Reconcile: failed to recover synthetics by range",
						"module", "synthetic", "source", source, "destination", me,
						"start", have+1, "end", overdue, "error", err)
				case done:
					mReconcile.WithLabelValues("succeeded").Inc()
					c.remoteOK(source.String())
					slog.WarnContext(ctx, "Reconcile: pulled messages a gap scan cannot see",
						"module", "synthetic", "source", source, "destination", me,
						"received", have, "produced", produced, "requested", overdue-have)
					continue
				}
			}
		}

		// Pull the missing range, capped like any other heal scan. The
		// per-(source,sequence) claim keeps N validators from all pulling the
		// same message.
		healed := 0
		for seq := have + 1; seq <= produced && healed < syntheticHealBatch; seq++ {
			// Wait for the gap to persist before believing it. Without this the
			// reconcile races normal delivery (#4073).
			if !c.gapIsOverdue(source, seq, blockIndex) {
				continue
			}
			// claimSequence, NOT claimSyntheticRequest. The latter keeps
			// per-source back-off around a single `want` value because #4064
			// heals one hole at a time; calling it across a range makes each
			// sequence clobber the previous one's state and they all back off
			// against each other. Observed: 21 of 26 overdue sequences silently
			// discarded, zero pulls, no error. claimSequence is keyed per
			// (source, sequence), which is what a range needs.
			if !c.claimSequence(source, seq, now, window) {
				continue
			}
			mReconcile.WithLabelValues("attempted").Inc()
			err := c.requestSyntheticFrom(ctx, source, seq)
			if err != nil {
				mReconcile.WithLabelValues("failed").Inc()
				c.classifyRemoteError(source.String(), err)
				slog.ErrorContext(ctx, "Reconcile: failed to request missing synthetic",
					"module", "synthetic", "source", source, "destination", me,
					"number", seq, "error", err)
				// "Reached the end of the chain" is the source saying it
				// cannot serve this sequence YET — its own anchoring has not
				// caught up. Every higher sequence is past the same end, so
				// walking the rest of the range converts one deterministic
				// "not yet" into a batch of guaranteed failures (#4086).
				if errors.Is(err, errors.NotFound) {
					break
				}
				continue
			}
			mReconcile.WithLabelValues("succeeded").Inc()
			c.remoteOK(source.String())
			healed++
		}
		if healed > 0 {
			// Warn, not Info: nodes drop Info at their default level, so this
			// logged nothing even when it worked — the recovery had to be
			// inferred from a control run. Pulling here means a message was
			// lost outright and would never have arrived, which is worth
			// surfacing.
			slog.WarnContext(ctx, "Reconcile: pulled messages a gap scan cannot see",
				"module", "synthetic", "source", source, "destination", me,
				"received", have, "produced", produced, "requested", healed)
		}
	}
	return nil
}

// gapIsOverdue reports whether (source, seq) has been missing long enough to be
// treated as lost rather than in flight. The first sighting only starts the
// clock; the caller must see the same gap still open reconcileGraceBlocks later.
// Entries for sequences that arrive on their own are dropped by pruneOverdue.
func (c *Conductor) gapIsOverdue(source *url.URL, seq uint64, blockIndex uint64) bool {
	key := source.String() + "#" + strconv.FormatUint(seq, 10)
	c.synthHealMu.Lock()
	defer c.synthHealMu.Unlock()
	if c.reconcileSeen == nil {
		c.reconcileSeen = map[string]uint64{}
	}
	first, ok := c.reconcileSeen[key]
	if !ok {
		c.reconcileSeen[key] = blockIndex
		return false
	}
	return blockIndex >= first+reconcileGraceBlocks
}

// pruneOverdue forgets gaps below the given received watermark, so the map does
// not grow without bound as streams advance normally.
func (c *Conductor) pruneOverdue(source *url.URL, received uint64) {
	prefix := source.String() + "#"
	c.synthHealMu.Lock()
	defer c.synthHealMu.Unlock()
	for k := range c.reconcileSeen {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		if n, err := strconv.ParseUint(k[len(prefix):], 10, 64); err == nil && n <= received {
			delete(c.reconcileSeen, k)
		}
	}
}

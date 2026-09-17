// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	dagconfig "gitlab.com/accumulatenetwork/accumulate/pkg/consensus/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The requesting side of healing (healing.md, "Deciding, in staging").
// Staging keeps, per stream, the entries received at their index and the
// hashes collection proofs have validated. Execution takes the validated
// prefix from Delivered upward, in order. Two things are gaps, and they are
// the only things healing asks for: an index a proof validated that staging
// does not hold, and entries staging holds that no proof has validated. On an
// activation block a selected validator walks the stream once, from Delivered
// to the highest entry held, coalesces both kinds into spans and asks the
// source for them; the answer -- the entries and the span's proof -- lands as
// a bundle. Nothing is timed and nothing is inferred: a hole is asked for when
// it is seen, and asked again only after healPatience activations without
// its answer landing.

const (
	// healPatience is how many activations pass before an asked span is asked
	// again. A hash is asked for once while the answer can still arrive.
	healPatience = 3

	// MaxRequestSpans bounds the spans one activation asks a source for. What
	// does not fit waits: the oldest indexes go first because delivery is in
	// order.
	MaxRequestSpans = 16

	// healHorizon bounds how far above Delivered the requester looks. It is
	// staging's sanity horizon, not the cache's.
	healHorizon = 3600

	// maxSourceBackoff bounds, in activations, how long a source whose
	// requests all failed is left alone.
	maxSourceBackoff = 8

	// probeAfter is how many consecutive activations a stream's Delivered
	// must sit still before the requester asks its source for anything.
	//
	// The probe exists for a package lost whole -- entries and proof
	// together -- which leaves nothing held and which nothing else would
	// ever see. But "nothing held above Delivered" is also the ordinary
	// state of a stream that has just drained, so probing on sight made a
	// healthy network heal continuously: run 20260915T211229Z, no faults
	// induced and nothing dropped, pulled 743,000 entries against 23,000
	// requests -- 56% of all traffic those streams had ever carried, to
	// cover ~1% that was in flight and arriving anyway (#4280).
	//
	// A lost package leaves the stream STUCK, so Delivered stops. Waiting
	// for that tells the two apart.
	//
	// The wait has to clear the time normal delivery takes, or the probe
	// wakes inside the pipeline and asks for entries that are merely on
	// their way. A block's synthetics do not leave until a Directory
	// receipt covering that block comes back, so the path is the
	// proof-path latency -- about eight seconds plus seven block
	// intervals. Measured on run 20260916T004735Z at 500 tps: synthetic
	// streams ran 13.7 to 29.3 seconds in flight. At four activations the
	// probe fired at sixteen blocks, inside that, and a clean network
	// still healed ~300 entries a minute: the source's cache held what it
	// had produced and not yet dispatched, so it answered, and healing
	// delivered what dispatch was about to. Eight activations is
	// thirty-two blocks, clear of the measured path, and still starts
	// real recovery inside a minute.
	//
	// The same rule answers holes, and for a stronger reason: **a stream
	// executes in order, with no gaps** (executor spec, invariant 1). So
	// while Delivered is moving, nothing below it is missing, and a hole
	// above it either fills before delivery reaches it or stops Delivered
	// when it does -- at which point this fires. Asking on sight healed
	// every transient reorder instead: after the probe was gated, a clean
	// network still pulled ~1,500 entries a minute, in runs averaging 49
	// consecutive numbers, all of them in flight (#4280).
	probeAfter = 8

	// strandedAfter is how many consecutive activations a stream's requests
	// must all come back NotFound — the span is past the source's cache —
	// before the stream is stranded: the back-off doubles to its cap over
	// the first four, and three more at the cap say the answer will not
	// change (healing spec, "Stranded streams").
	strandedAfter = 7
)

var mStrandedStreams = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "accumulate",
	Subsystem: "conductor",
	Name:      "stranded_streams",
	Help:      "Streams whose oldest gap the source cannot serve (past its cache) and which are no longer asked for; sync is the way out",
}, []string{"destination", "source"})

var mHealRequests = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "conductor",
	Name:      "heal_requests_total",
	Help:      "Span requests by outcome: answered, not-yet (the span is in flight at the source), miss (the source's cache lacks the span and this node is caught up), lagging-miss (the source lacks it but this node is behind consensus, so it may be in its own backlog: asked again later, not a miss), failed, lagging (not asked: this node's executor is more than MaxExecutionLag behind consensus, #4260, #4284)",
}, []string{"outcome", "destination", "source"})

// An entryOutcome says what became of one entry an answer carried: whether it
// filled a gap or was redundant. It is decided against the destination's own
// position, so it is decided where that position is known (#4283).
type entryOutcome func(number uint64) string

var mHealEntries = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "conductor",
	Name:      "heal_entries_total",
	Help:      "Entries received in answer to span requests, by what became of them: applied (above the destination's delivered point and not already held, so the answer filled a gap) or not-required (at or below delivered, or already held, so the answer was redundant). A bare count of entries received cannot tell a repaired stream from a healer asking for what it already has, and cannot tell either from nothing to do (#4283)",
}, []string{"outcome", "destination", "source"})

// askedSpan is what the requester remembers about a span it asked for: the
// block it asked at. One record per span, not per index — a span is up to
// MaxReceiptListElements indexes, and a heap object per index was the
// requester's largest allocation (review 2026-09-06, finding 9). Node state,
// not consensus state; a restart empties it at the cost of one duplicate
// request.
type askedSpan struct {
	first, last, at uint64
}

type healRequester struct {
	mu       sync.Mutex
	asks     map[string][]askedSpan // stream -> spans asked within patience
	backoff  map[string]uint64      // source -> block before which it is not asked
	failures map[string]uint
	misses   map[string]uint       // stream -> consecutive activations answered only NotFound
	stranded map[string]strandedAt // stream -> where it was stranded
	still    map[string]stillAt    // stream -> how long Delivered has sat still
}

// stillAt is where a stream's Delivered stood when it stopped moving, and
// how many activations it has sat there. A stream whose Delivered moves is
// delivering and needs nothing; one whose Delivered sits still is the only
// kind healing is for (#4280).
type stillAt struct {
	delivered   uint64
	activations uint
}

// strandedAt is where a stream stood when it was stranded: Delivered then.
// The stream leaves the state when Delivered moves past it — something
// arrived that healing could not fetch, which is sync (E11).
type strandedAt struct {
	delivered uint64
}

// askedRecently reports whether n was asked within the last healPatience
// activations; the caller holds r.mu.
func askedRecently(asks []askedSpan, n, blockIndex uint64) bool {
	for _, a := range asks {
		if n >= a.first && n <= a.last && blockIndex-a.at < healPatience*healCadence {
			return true
		}
	}
	return false
}

func streamKey(id execute.StreamID) string {
	return strings.ToLower(id.Ledger.String() + "|" + id.Source.String())
}

// selectedToPull answers whether this validator is one of the pair the
// previous block's hash selects over the partition's validator set. Pulls are
// fungible — whoever asks, the answer heals everyone — so two ask and the rest
// stay quiet. A node with no validator key, or one not in the set, never pulls.
func (c *Conductor) selectedToPull(ledger *protocol.SystemLedger) bool {
	if len(c.ValidatorKey) != ed25519.PrivateKeySize {
		return false
	}
	keys, err := c.partitionValidators()
	if err != nil || len(keys) == 0 {
		return false
	}
	self := c.ValidatorKey.Public().(ed25519.PublicKey)
	me := -1
	for i, k := range keys {
		if bytes.Equal(k, self) {
			me = i
			break
		}
	}
	if me < 0 {
		return false
	}
	for _, i := range pullSenders(previousBlockSeed(ledger), len(keys)) {
		if i == me {
			return true
		}
	}
	return false
}

// previousBlockSeed is the agreed hash the selection is drawn from: the root
// anchor of the last block, which every validator holds and nobody chooses.
func previousBlockSeed(ledger *protocol.SystemLedger) []byte {
	if ledger != nil && ledger.Anchor != nil {
		if pa := ledger.Anchor.GetPartitionAnchor(); pa != nil {
			return pa.RootChainAnchor[:]
		}
	}
	var b [8]byte
	if ledger != nil {
		binary.BigEndian.PutUint64(b[:], ledger.Index)
	}
	return b[:]
}

// pullSenders picks sendersPerActivation distinct positions in [0, n) from the
// seed. Deterministic, so every validator agrees who asks.
func pullSenders(seed []byte, n int) []int {
	if n <= 0 {
		return nil
	}
	h := sha256.Sum256(append([]byte("heal-pull:"), seed...))
	first := int(binary.BigEndian.Uint64(h[:8]) % uint64(n))
	if n == 1 || sendersPerActivation == 1 {
		return []int{first}
	}
	second := (first + 1 + int(binary.BigEndian.Uint64(h[8:16])%uint64(n-1))) % n
	return []int{first, second}
}

// inboundSources lists every partition that sends synthetic messages to this
// one: every other partition of the network, plus any the ledger already
// tracks. Sorted, so every validator visits them in the same order.
func (c *Conductor) inboundSources(ledger *protocol.SyntheticLedger) []*url.URL {
	seen := map[string]bool{}
	var sources []*url.URL
	add := func(u *url.URL) {
		if u == nil || strings.EqualFold(u.String(), c.Url().String()) || seen[strings.ToLower(u.String())] {
			return
		}
		seen[strings.ToLower(u.String())] = true
		sources = append(sources, u)
	}
	if g := c.Globals.Load(); g != nil && g.Network != nil {
		for _, peer := range g.Network.Partitions {
			add(protocol.PartitionUrl(peer.ID))
		}
	}
	if ledger != nil {
		for _, part := range ledger.Sequence {
			add(part.Url)
		}
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].String() < sources[j].String() })
	return sources
}

// requestGaps is one activation: decide per source from staging, ask, and
// submit what comes back. It runs outside consensus, bounded by HealTimeout.
func (c *Conductor) requestGaps(ctx context.Context, batch *database.Batch, blockIndex uint64) error {
	ranger, ok := c.Sequencer.(private.SequenceRanger)
	if !ok {
		return nil
	}
	var synth *protocol.SyntheticLedger
	err := batch.Account(c.Url(protocol.Synthetic)).Main().GetAs(&synth)
	if err != nil {
		return errors.UnknownError.WithFormat("load synthetic ledger: %w", err)
	}
	var anchors *protocol.AnchorLedger
	err = batch.Account(c.Url(protocol.AnchorPool)).Main().GetAs(&anchors)
	if err != nil {
		return errors.UnknownError.WithFormat("load anchor ledger: %w", err)
	}

	staged := c.Staging.Begin()
	defer staged.Discard()

	// Every stream is its own stage and is asked about on its own (executor
	// spec, "One chain per pair, one stage per chain"): the synthetic stream
	// from each source, and the anchor stream from each partition that
	// anchors here.
	for _, source := range c.inboundSources(synth) {
		c.requestStream(ctx, staged, blockIndex, source, streamAsk{
			stream:    execute.StreamID{Ledger: c.Url(protocol.Synthetic), Source: source},
			delivered: synth.Partition(source).Delivered,
			what:      "synthetics",
			ask: func(first, last uint64, classify entryOutcome) (int, uint64, error) {
				return c.requestSpan(ctx, ranger, source, first, last, classify)
			},
			healed: func(n int) {
				if c.Heals != nil {
					c.Heals.Synthetic.Add(uint64(n))
				}
				c.synthHeals.Add(uint64(n))
			},
		})
	}
	for _, source := range c.anchorSources() {
		c.requestStream(ctx, staged, blockIndex, source.JoinPath(protocol.AnchorPool), streamAsk{
			stream:    execute.StreamID{Ledger: c.Url(protocol.AnchorPool), Source: source},
			delivered: anchors.Partition(source).Delivered,
			what:      "anchors",
			ask: func(first, last uint64, classify entryOutcome) (int, uint64, error) {
				return c.requestAnchorSpan(ctx, ranger, source, first, last, classify)
			},
			healed: func(n int) {
				if c.Heals != nil {
					c.Heals.Anchor.Add(uint64(n))
				}
			},
		})
	}

	// Bundles were submitted to the dispatcher by this task, after the block
	// hook's own Send ran. Send them now rather than a block later.
	for err := range c.Dispatcher.Send(ctx) {
		slog.ErrorContext(ctx, "Failed to dispatch healing bundle", "module", "conductor", "error", err)
	}
	return nil
}

// anchorSources lists the partitions whose anchors this partition executes:
// the Directory for a BVN; every partition, itself included, for the Directory.
func (c *Conductor) anchorSources() []*url.URL {
	if c.Partition.Type != protocol.PartitionTypeDirectory {
		return []*url.URL{protocol.DnUrl()}
	}
	var sources []*url.URL
	if g := c.Globals.Load(); g != nil && g.Network != nil {
		for _, part := range g.Network.Partitions {
			sources = append(sources, protocol.PartitionUrl(part.ID))
		}
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].String() < sources[j].String() })
	return sources
}

// streamAsk is one stream's part of an activation: where it stands, how to
// ask its source for a span, and how to count what came back.
type streamAsk struct {
	stream    execute.StreamID
	delivered uint64
	what      string
	ask       func(first, last uint64, classify entryOutcome) (int, uint64, error)

	healed func(n int)
}

// requestStream decides one stream's gaps from staging, asks for them, and
// records each outcome. backoff names the source for the per-source back-off;
// a source's two streams back off independently.
func (c *Conductor) requestStream(ctx context.Context, staged *execute.StagingTxn, blockIndex uint64, backoff *url.URL, a streamAsk) {
	source := a.stream.Source
	if left := c.requester.strandedStream(a.stream, a.delivered); left == strandedStill {
		return
	} else if left == strandedLeft {
		mStrandedStreams.WithLabelValues(c.Partition.ID, partitionLabel(source)).Set(0)
		slog.WarnContext(ctx, "Stream is no longer stranded", "module", "conductor",
			"source", source, "destination", c.Url(), "what", a.what, "delivered", a.delivered, "block", blockIndex)
	}
	if c.requester.backedOff(backoff, blockIndex) {
		return
	}
	// A node whose executor is lagging consensus -- more than MaxExecutionLag
	// groups behind, the node's one definition of lagging -- decides from a
	// staging that is behind too: what it lacks is in its own unexecuted
	// blocks, and nothing is lost by waiting for them to run (#4260). Within
	// that window it asks; a real hole must be asked for or it is permanent,
	// and a NotFound taken while behind is handled below as not-yet rather
	// than as a miss (#4284).
	if c.lagging() {
		mHealRequests.WithLabelValues("lagging", c.Partition.ID, partitionLabel(source)).Inc()
		return
	}
	spans := c.requester.decide(staged, a.stream, a.delivered, blockIndex)
	if len(spans) == 0 {
		return
	}
	asked, failed, missed := 0, 0, 0
	for _, span := range spans {
		if ctx.Err() != nil {
			break
		}
		// What became of each entry is decided here, where the stream's
		// delivered point and the stage are both in hand (#4283).
		classify := func(number uint64) string {
			if number <= a.delivered {
				return "not-required" // already executed
			}
			if _, held := staged.IDOf(a.stream, number); held {
				return "not-required" // already in the stage, just not run yet
			}
			return "applied"
		}
		n, served, err := a.ask(span[0], span[1], classify)
		switch {
		case err == nil:
			asked++
			c.requester.asked(a.stream, [2]uint64{span[0], served}, blockIndex)
			mHealRequests.WithLabelValues("answered", c.Partition.ID, partitionLabel(source)).Inc()
			if c.Heals != nil {
				c.Heals.Requests.Add(1)
			}
			a.healed(n)
			slog.WarnContext(ctx, "Requested missing "+a.what, "module", "conductor",
				"source", source, "destination", c.Url(), "start", span[0], "end", span[1], "entries", n, "block", blockIndex)
		case errors.Is(err, errors.NotReady):
			// The source has not dispatched the span, or dispatched it
			// within the last few blocks: the entries are on their way.
			// Not a gap yet, not a failure. Remembered like an answer, so a
			// quiet stream's probe fires once per patience window rather
			// than every activation.
			c.requester.asked(a.stream, span, blockIndex)
			mHealRequests.WithLabelValues("not-yet", c.Partition.ID, partitionLabel(source)).Inc()
			slog.InfoContext(ctx, "Missing "+a.what+" are still in flight at the source", "module", "conductor",
				"source", source, "destination", c.Url(), "start", span[0], "end", span[1], "error", err)
		case errors.Is(err, errors.NotFound) && c.executionLagBlocks() > 0:
			// The source does not hold the span, and this node's executor is
			// behind consensus: the span may sit in its own committed,
			// unexecuted blocks, released at the source on the partition's
			// Delivered, which is ahead of this node (#4260). That is not
			// evidence of a defect at the source. Remembered like an answer
			// and asked again once the backlog has run; it does not count
			// toward stranding, which is decided on misses taken while
			// caught up (#4284).
			c.requester.asked(a.stream, span, blockIndex)
			mHealRequests.WithLabelValues("lagging-miss", c.Partition.ID, partitionLabel(source)).Inc()
			slog.InfoContext(ctx, "Source does not hold missing "+a.what+" and this node is behind consensus; asking again after the backlog", "module", "conductor",
				"source", source, "destination", c.Url(), "start", span[0], "end", span[1], "lag", c.executionLagBlocks(), "error", err)
		case errors.Is(err, errors.NotFound):
			// The source's cache does not hold the span, and this node is
			// caught up, so the span is not in its own backlog. Deterministic:
			// asking again does not help. A miss is a defect at the source.
			failed++
			missed++
			mHealRequests.WithLabelValues("miss", c.Partition.ID, partitionLabel(source)).Inc()
			if c.Heals != nil {
				c.Heals.Requests.Add(1)
				c.Heals.Misses.Add(1)
			}
			slog.ErrorContext(ctx, "Source cannot serve missing "+a.what, "module", "conductor",
				"source", source, "destination", c.Url(), "start", span[0], "end", span[1], "error", err)
		default:
			failed++
			mHealRequests.WithLabelValues("failed", c.Partition.ID, partitionLabel(source)).Inc()
			if c.Heals != nil {
				c.Heals.Requests.Add(1)
			}
			slog.ErrorContext(ctx, "Failed to request missing "+a.what, "module", "conductor",
				"source", source, "destination", c.Url(), "start", span[0], "end", span[1], "error", err)
		}
	}
	c.requester.outcome(backoff, blockIndex, asked, failed)
	if c.requester.streamOutcome(a.stream, a.delivered, missed, missed == failed && asked == 0) {
		mStrandedStreams.WithLabelValues(c.Partition.ID, partitionLabel(source)).Set(1)
		slog.WarnContext(ctx, "Stream is stranded: the source cannot serve its oldest gap and will not be asked again; sync is the way out", "module", "conductor",
			"source", source, "destination", c.Url(), "what", a.what, "delivered", a.delivered, "block", blockIndex)
	}
}

// strandedState is what strandedStream answers about a stream.
type strandedState int

const (
	notStranded   strandedState = iota
	strandedStill               // stranded, and nothing has moved: not asked
	strandedLeft                // was stranded, and Delivered has moved past it
)

// strandedStream answers whether a stream is stranded, and takes it out of
// the state when its Delivered has moved past where it was stranded: the hole
// was filled by something other than a request, which is sync.
func (r *healRequester) strandedStream(stream execute.StreamID, delivered uint64) strandedState {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := streamKey(stream)
	at, ok := r.stranded[k]
	switch {
	case !ok:
		return notStranded
	case delivered > at.delivered:
		delete(r.stranded, k)
		delete(r.misses, k)
		return strandedLeft
	default:
		return strandedStill
	}
}

// streamOutcome counts an activation whose every request for the stream came
// back NotFound, and strands the stream once strandedAfter of them are
// consecutive. Anything else the source answers for the stream — entries, or
// "not yet" — resets the count. Answers whether the stream was stranded by
// this call, so the caller says so once.
func (r *healRequester) streamOutcome(stream execute.StreamID, delivered uint64, missed int, onlyMissed bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.misses == nil {
		r.misses = map[string]uint{}
		r.stranded = map[string]strandedAt{}
	}
	k := streamKey(stream)
	if missed == 0 || !onlyMissed {
		delete(r.misses, k)
		return false
	}
	r.misses[k]++
	if r.misses[k] < strandedAfter {
		return false
	}
	if _, ok := r.stranded[k]; ok {
		return false
	}
	r.stranded[k] = strandedAt{delivered}
	return true
}

// Stranded lists the streams the requester has given up asking for, by key.
func (r *healRequester) Stranded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.stranded))
	for k := range r.stranded {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// decide walks one stream from Delivered to the highest entry held and
// returns the spans to ask for: indexes not held, and entries held without a
// validating hash. Consecutive gaps of either kind coalesce; at most
// MaxRequestSpans spans, each within MaxReceiptListElements, oldest first.
// A span asked within the last healPatience activations is not asked again.
// One pass, two map lookups per index, no allocation beyond the spans.
func (r *healRequester) decide(staged *execute.StagingTxn, stream execute.StreamID, delivered, blockIndex uint64) [][2]uint64 {
	// Healing is for a synthetic stream that has STOPPED, not one that is
	// moving. Delivery is in order, so a moving Delivered proves nothing
	// below it is missing, and whatever is missing above it will stop
	// Delivered when delivery reaches it (#4280). This gates everything
	// below.
	//
	// SYNTHETIC streams only. Every measurement behind this rule came from
	// one -- the 743,000-entry storm, the runs averaging 49 consecutive
	// numbers, the 13.7-29.3s in-flight times -- and an anchor stream is a
	// different shape: roughly one entry per block per partition, executed
	// under a quorum, with none of the constant drain that makes an empty
	// synthetic stream ambiguous. Gating anchors too was a generalisation
	// with no evidence under it, and it did not merely slow recovery of a
	// lost block validator anchor, it prevented it:
	// TestMissingBlockValidatorAnchorTxn went from 1 failure in 20 runs to
	// 13, and stayed broken with a 600-block budget. An anchor stream is
	// asked on sight.
	if isSyntheticStream(stream) && !r.stillLongEnough(streamKey(stream), delivered) {
		return nil
	}

	// The walk below runs to whichever list reaches further: entries beyond
	// the validated hashes are a gap of proof, validated hashes beyond the
	// entries are a gap of entries.
	sighted := staged.Sighted(stream)
	if reach := staged.Reach(stream); reach > sighted {
		sighted = reach
	}
	r.forget(stream, delivered)
	if sighted <= delivered {
		// Nothing held above Delivered. A lost package -- entries and proof
		// together -- leaves exactly this and nothing else would ever see
		// it, so the span above Delivered is asked for whole. But a stream
		// that has merely drained looks identical at this instant, so the
		// probe waits for Delivered to STOP moving first (probeAfter,
		// #4280); a lost package stops it, normal delivery does not.
		r.mu.Lock()
		defer r.mu.Unlock()
		if askedRecently(r.asks[streamKey(stream)], delivered+1, blockIndex) {
			return nil
		}
		return [][2]uint64{{delivered + 1, delivered + protocol.MaxReceiptListElements}}
	}
	through := sighted
	if through > delivered+healHorizon {
		through = delivered + healHorizon
	}

	// A collected entry whose proof has arrived and waits for its Directory
	// anchor is not a gap: the anchor is on its way, late when this
	// partition's executor lags, and asking the source again lands the entry
	// twice (run 20260905T134346Z: 22,642 heals with nothing dropped; the
	// C6/H6 storm). Proofs wait per source and cover the synthetic chain, so
	// only the synthetic stream reads them.
	var waiting [][2]uint64
	if isSyntheticStream(stream) {
		waiting = staged.StagedProofSpans(stream.Source)
	}
	proofWaiting := func(n uint64) bool {
		for _, w := range waiting {
			if n >= w[0] && n <= w[1] {
				return true
			}
		}
		return false
	}

	// A number needs nothing from the source when staging already accounts
	// for it, in any of the three ways it can: held by the sequenced layer
	// rather than awaiting a proof, held with its hash already validated, or
	// held with its proof staged and only its anchor outstanding.
	accountedFor := func(n uint64) bool {
		h, held := staged.IDOf(stream, n)
		if !held {
			return false
		}
		return !h.Collected || staged.IsValidated(stream, n, h.Hash) || proofWaiting(n)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	asks := r.asks[streamKey(stream)]
	var spans [][2]uint64
	for n := delivered + 1; n <= through; n++ {
		if accountedFor(n) {
			continue
		}
		if askedRecently(asks, n, blockIndex) {
			continue // asked; its answer can still land
		}
		if k := len(spans); k > 0 && spans[k-1][1]+1 == n && n-spans[k-1][0]+1 <= protocol.MaxReceiptListElements {
			spans[k-1][1] = n
			continue
		}
		if len(spans) == MaxRequestSpans {
			break
		}
		spans = append(spans, [2]uint64{n, n})
	}
	return spans
}

// isSyntheticStream reports whether a stream is a synthetic stream, as
// opposed to an anchor stream: the ledger that tracks it is the partition's
// synthetic ledger.
func isSyntheticStream(id execute.StreamID) bool {
	return id.Ledger != nil && strings.EqualFold(strings.Trim(id.Ledger.Path, "/"), protocol.Synthetic)
}

// forget drops what a stream's memory holds at or below Delivered, and what
// has aged past patience; what was asked above Delivered keeps its asked-at.
// stillLongEnough counts one activation on which a stream's Delivered did
// not move, and reports whether it has now sat still for probeAfter of them.
// Delivered moving resets the count: the stream is delivering, which is the
// opposite of every case healing exists for (#4280).
func (r *healRequester) stillLongEnough(key string, delivered uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.still == nil {
		r.still = map[string]stillAt{}
	}
	at, ok := r.still[key]
	if !ok || at.delivered != delivered {
		r.still[key] = stillAt{delivered: delivered, activations: 1}
		return false
	}
	at.activations++
	r.still[key] = at
	return at.activations >= probeAfter
}

func (r *healRequester) forget(stream execute.StreamID, delivered uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := streamKey(stream)
	if len(r.asks[k]) == 0 {
		return
	}
	kept := r.asks[k][:0]
	for _, a := range r.asks[k] {
		if a.last <= delivered {
			continue
		}
		if a.first <= delivered {
			a.first = delivered + 1
		}
		kept = append(kept, a)
	}
	clear(r.asks[k][len(kept):])
	r.asks[k] = kept
}

// asked records that the span was asked on this block. Spans that can no
// longer answer "asked recently" are dropped, so the memory holds at most a
// patience window of spans per stream.
func (r *healRequester) asked(stream execute.StreamID, span [2]uint64, blockIndex uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.asks == nil {
		r.asks = map[string][]askedSpan{}
	}
	k := streamKey(stream)
	kept := r.asks[k][:0]
	for _, a := range r.asks[k] {
		if blockIndex-a.at < healPatience*healCadence {
			kept = append(kept, a)
		}
	}
	clear(r.asks[k][len(kept):])
	r.asks[k] = append(kept, askedSpan{first: span[0], last: span[1], at: blockIndex})
}

func (r *healRequester) backedOff(source *url.URL, blockIndex uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.backoff[sourceKey(source)] > blockIndex
}

// outcome backs a source off when every request to it failed — one activation
// after the first failure, doubling to maxSourceBackoff — and clears the
// back-off when it answers again.
func (r *healRequester) outcome(source *url.URL, blockIndex uint64, asked, failed int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.backoff == nil {
		r.backoff = map[string]uint64{}
		r.failures = map[string]uint{}
	}
	k := sourceKey(source)
	if asked > 0 || failed == 0 {
		delete(r.backoff, k)
		delete(r.failures, k)
		return
	}
	r.failures[k]++
	wait := uint64(1) << (r.failures[k] - 1)
	if wait > maxSourceBackoff {
		wait = maxSourceBackoff
	}
	r.backoff[k] = blockIndex + wait*healCadence
}

func sourceKey(u *url.URL) string { return strings.ToLower(u.String()) }

// requestSpan asks the source for [first, last] of its synthetic stream to
// this partition and submits the answer — the entries, each with its
// companion transaction when it has one, under one collection proof — as
// bundles within the envelope budget. The source may answer a prefix of the
// span — what it has dispatched and is not still in flight — so the result
// says how many entries were submitted and the last number among them.
func (c *Conductor) requestSpan(ctx context.Context, ranger private.SequenceRanger, source *url.URL, first, last uint64, classify entryOutcome) (int, uint64, error) {
	records, err := ranger.SequenceRange(ctx, source.JoinPath(protocol.Synthetic), c.Url(), first, last, private.SequenceOptions{})
	if err != nil {
		return 0, 0, errors.UnknownError.Wrap(err)
	}
	if len(records) == 0 {
		return 0, 0, errors.InvalidRecord.With("empty answer")
	}
	tail := records[len(records)-1]
	if tail.Sequence == nil {
		return 0, 0, errors.InvalidRecord.With("answer carries an unsequenced message")
	}
	if tail.SourceReceiptList == nil {
		return 0, 0, errors.InvalidRecord.With("answer carries no collection proof")
	}
	if tail.SourceAnchorBlock == 0 {
		return 0, 0, errors.InvalidRecord.With("answer does not say which Directory block proves it")
	}
	served := tail.Sequence.Number
	proof := &protocol.AnnotatedReceipt{
		ReceiptList: tail.SourceReceiptList,
		Anchor:      &protocol.AnchorMetadata{Account: protocol.DnUrl(), SourceBlock: tail.SourceAnchorBlock},
	}
	proofMsg := &messaging.SyntheticProof{Proof: proof}
	proofSize, err := marshalledSize(proofMsg)
	if err != nil {
		return 0, 0, err
	}

	baikonur := c.Globals.Load().ExecutorVersion.V2BaikonurEnabled()
	budget := dagconfig.DefaultMaxBatchBytes - dagconfig.DefaultMaxBatchBytes/4
	var msgs []messaging.Message
	size := 0
	flush := func() error {
		if len(msgs) == 0 {
			return nil
		}
		env := &messaging.Envelope{Messages: append([]messaging.Message{proofMsg}, msgs...)}
		err := c.submit(ctx, c.Url(), env)
		msgs, size = nil, 0
		return err
	}
	for _, r := range records {
		if r.Sequence == nil {
			return 0, 0, errors.InvalidRecord.With("answer carries an unsequenced message")
		}
		keySig := keySignatureOf(r)
		if keySig == nil {
			return 0, 0, errors.InvalidRecord.WithFormat("answer for %v→%v #%d is not signed", source, c.Url(), r.Sequence.Number)
		}
		mHealEntries.WithLabelValues(classify(r.Sequence.Number), c.Partition.ID, partitionLabel(source)).Inc()
		var entry messaging.Message
		if baikonur {
			entry = &messaging.SyntheticMessage{Message: r.Sequence, Signature: keySig}
		} else {
			entry = &messaging.BadSyntheticMessage{Message: r.Sequence, Signature: keySig}
		}
		add := []messaging.Message{entry}
		if r.Companion != nil {
			add = append(add, r.Companion)
		}
		n := 0
		for _, m := range add {
			k, err := marshalledSize(m)
			if err != nil {
				return 0, 0, err
			}
			n += k
		}
		if len(msgs) > 0 && proofSize+size+n > budget {
			if err := flush(); err != nil {
				return 0, 0, errors.UnknownError.WithFormat("submit bundle from %v: %w", source, err)
			}
		}
		msgs = append(msgs, add...)
		size += n
	}
	if err := flush(); err != nil {
		return 0, 0, errors.UnknownError.WithFormat("submit bundle from %v: %w", source, err)
	}
	return len(records), served, nil
}

// requestAnchorSpan asks the source for anchors [first, last] of its stream to
// this partition and submits them as BlockAnchors, one per signature the
// source holds: together they are the quorum, and each is what that
// validator's own dispatch carried. An anchor's signatures travel in ONE
// envelope, and as many anchors as fit the envelope budget share it — the
// block sorts the envelope under the anchor's number and processes every
// message in it, so each copy records its signature (review 2026-09-06,
// finding 9: one envelope per signature per record). The source may answer a
// prefix; the number it served through is returned.
func (c *Conductor) requestAnchorSpan(ctx context.Context, ranger private.SequenceRanger, source *url.URL, first, last uint64, classify entryOutcome) (int, uint64, error) {
	records, err := c.anchorAnswers(ctx, ranger, source, first, last)
	if err != nil {
		return 0, 0, errors.UnknownError.Wrap(err)
	}
	if len(records) == 0 {
		return 0, 0, errors.InvalidRecord.With("empty answer")
	}
	budget := dagconfig.DefaultMaxBatchBytes - dagconfig.DefaultMaxBatchBytes/4
	var msgs []messaging.Message
	size := 0
	flush := func() error {
		if len(msgs) == 0 {
			return nil
		}
		err := c.submit(ctx, c.Url(), &messaging.Envelope{Messages: msgs})
		msgs, size = nil, 0
		return err
	}
	var served uint64
	for _, r := range records {
		if r.Sequence == nil {
			return 0, 0, errors.InvalidRecord.With("answer carries an unsequenced message")
		}
		sigs := keySignaturesOf(r)
		if len(sigs) == 0 {
			return 0, 0, errors.InvalidRecord.WithFormat("answer for anchor %v→%v #%d is not signed", source, c.Url(), r.Sequence.Number)
		}
		mHealEntries.WithLabelValues(classify(r.Sequence.Number), c.Partition.ID, partitionLabel(source)).Inc()
		var add []messaging.Message
		n := 0
		for _, sig := range sigs {
			m := &messaging.BlockAnchor{Anchor: r.Sequence, Signature: sig}
			k, err := marshalledSize(m)
			if err != nil {
				return 0, 0, err
			}
			add = append(add, m)
			n += k
		}
		if len(msgs) > 0 && size+n > budget {
			if err := flush(); err != nil {
				return 0, 0, errors.UnknownError.WithFormat("submit anchors from %v: %w", source, err)
			}
		}
		msgs = append(msgs, add...)
		size += n
		served = r.Sequence.Number
	}
	if err := flush(); err != nil {
		return 0, 0, errors.UnknownError.WithFormat("submit anchors from %v: %w", source, err)
	}
	return len(records), served, nil
}

func marshalledSize(m messaging.Message) (int, error) {
	b, err := m.MarshalBinary()
	if err != nil {
		return 0, errors.UnknownError.WithFormat("marshal %v: %w", m.Type(), err)
	}
	return len(b), nil
}

// keySignatureOf finds the source validator's signature over the sequenced
// message in a sequencer record.
func keySignatureOf(r *api.MessageRecord[messaging.Message]) protocol.KeySignature {
	sigs := keySignaturesOf(r)
	if len(sigs) == 0 {
		return nil
	}
	return sigs[0]
}

// anchorAnswers gathers a quorum for an anchor span. An anchor executes at
// the destination under a validator signature quorum, and one node's answer
// carries that node's signature (a BVN never holds its own anchor with the
// others' signatures, so it has nothing more to give). One unaddressed
// request therefore yields one signer, and which signer is whatever the
// transport happened to dial: the spec's "successive activations gather
// distinct signatures" was true only by luck, and a completely lost block
// validator anchor stayed lost about one run in thirty.
//
// So the requester asks the source's validators one by one, by node, and
// merges their signatures per anchor until the first anchor in the span --
// the one the stream is waiting on -- has a quorum of distinct signers. With
// no way to find peers it asks once, as before.
func (c *Conductor) anchorAnswers(ctx context.Context, ranger private.SequenceRanger, source *url.URL, first, last uint64) ([]*api.MessageRecord[messaging.Message], error) {
	src := source.JoinPath(protocol.AnchorPool)
	records, err := ranger.SequenceRange(ctx, src, c.Url(), first, last, private.SequenceOptions{})
	if err != nil {
		return nil, err
	}
	if len(records) == 0 || c.Peers == nil {
		return records, nil
	}

	threshold := 1
	if g := c.Globals.Load(); g != nil {
		if id, ok := protocol.ParsePartitionUrl(source); ok {
			if t := g.ValidatorThreshold(id); t > 0 && t < 1<<20 {
				threshold = int(t)
			}
		}
	}
	signers := func(r *api.MessageRecord[messaging.Message]) map[string]bool {
		m := map[string]bool{}
		for _, sig := range keySignaturesOf(r) {
			m[string(sig.GetPublicKey())] = true
		}
		return m
	}
	if len(signers(records[0])) >= threshold {
		return records, nil
	}

	peers, err := c.Peers.FindService(ctx, api.FindServiceOptions{Service: private.ServiceTypeSequencer.AddressFor(c.partitionOf(source))})
	if err != nil {
		slog.WarnContext(ctx, "Cannot find the source's sequencers; one answer will have to do", "module", "conductor", "source", source, "error", err)
		return records, nil
	}
	byNumber := map[uint64]*api.MessageRecord[messaging.Message]{}
	for _, r := range records {
		if r.Sequence != nil {
			byNumber[r.Sequence.Number] = r
		}
	}
	have := signers(records[0])
	for _, p := range peers {
		if len(have) >= threshold {
			break
		}
		more, err := ranger.SequenceRange(ctx, src, c.Url(), first, last, private.SequenceOptions{NodeID: p.PeerID})
		if err != nil {
			continue // one validator's silence is not the answer's
		}
		for _, r := range more {
			if r.Sequence == nil {
				continue
			}
			base, ok := byNumber[r.Sequence.Number]
			if !ok {
				byNumber[r.Sequence.Number] = r
				records = append(records, r)
				continue
			}
			seen := signers(base)
			for _, set := range r.Signatures.Records {
				for _, m := range set.Signatures.Records {
					sm, ok := m.Message.(*messaging.SignatureMessage)
					if !ok {
						continue
					}
					ks, ok := sm.Signature.(protocol.KeySignature)
					if !ok || seen[string(ks.GetPublicKey())] {
						continue
					}
					seen[string(ks.GetPublicKey())] = true
					base.Signatures.Records = append(base.Signatures.Records, set)
					base.Signatures.Total++
				}
			}
		}
		have = signers(records[0])
	}
	return records, nil
}

// partitionOf is the partition ID a partition URL names, or the URL's
// authority when it is not one.
func (c *Conductor) partitionOf(u *url.URL) string {
	if id, ok := protocol.ParsePartitionUrl(u); ok {
		return id
	}
	return u.Authority
}

// keySignaturesOf lists every key signature an answer carries, one per signer.
func keySignaturesOf(r *api.MessageRecord[messaging.Message]) []protocol.KeySignature {
	if r.Signatures == nil {
		return nil
	}
	var sigs []protocol.KeySignature
	seen := map[string]bool{}
	for _, set := range r.Signatures.Records {
		if set == nil || set.Signatures == nil {
			continue
		}
		for _, s := range set.Signatures.Records {
			sm, ok := s.Message.(*messaging.SignatureMessage)
			if !ok {
				continue
			}
			ks, ok := sm.Signature.(protocol.KeySignature)
			if !ok || seen[string(ks.GetPublicKey())] {
				continue
			}
			seen[string(ks.GetPublicKey())] = true
			sigs = append(sigs, ks)
		}
	}
	return sigs
}

func partitionLabel(u *url.URL) string {
	if id, ok := protocol.ParsePartitionUrl(u); ok {
		return id
	}
	return u.ShortString()
}

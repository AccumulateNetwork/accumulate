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
)

var mHealRequests = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "conductor",
	Name:      "heal_requests_total",
	Help:      "Synthetic span requests by outcome: answered, not-yet (the span is in flight at the source), miss (the source's cache lacks the span), failed",
}, []string{"outcome", "destination", "source"})

var mHealEntries = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "accumulate",
	Subsystem: "conductor",
	Name:      "heal_entries_total",
	Help:      "Synthetic entries received in answer to span requests",
})

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
			ask: func(first, last uint64) (int, uint64, error) {
				return c.requestSpan(ctx, ranger, source, first, last)
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
			ask: func(first, last uint64) (int, uint64, error) {
				return c.requestAnchorSpan(ctx, ranger, source, first, last)
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
	ask       func(first, last uint64) (int, uint64, error)
	healed    func(n int)
}

// requestStream decides one stream's gaps from staging, asks for them, and
// records each outcome. backoff names the source for the per-source back-off;
// a source's two streams back off independently.
func (c *Conductor) requestStream(ctx context.Context, staged *execute.StagingTxn, blockIndex uint64, backoff *url.URL, a streamAsk) {
	if c.requester.backedOff(backoff, blockIndex) {
		return
	}
	source := a.stream.Source
	spans := c.requester.decide(staged, a.stream, a.delivered, blockIndex)
	if len(spans) == 0 {
		return
	}
	asked, failed := 0, 0
	for _, span := range spans {
		if ctx.Err() != nil {
			break
		}
		n, served, err := a.ask(span[0], span[1])
		switch {
		case err == nil:
			asked++
			c.requester.asked(a.stream, [2]uint64{span[0], served}, blockIndex)
			mHealRequests.WithLabelValues("answered", c.Partition.ID, partitionLabel(source)).Inc()
			mHealEntries.Add(float64(n))
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
		case errors.Is(err, errors.NotFound):
			// The source's cache does not hold the span. Deterministic:
			// asking again does not help. A miss is a defect at the source.
			failed++
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
}

// decide walks one stream from Delivered to the highest entry held and
// returns the spans to ask for: indexes not held, and entries held without a
// validating hash. Consecutive gaps of either kind coalesce; at most
// MaxRequestSpans spans, each within MaxReceiptListElements, oldest first.
// A span asked within the last healPatience activations is not asked again.
// One pass, two map lookups per index, no allocation beyond the spans.
func (r *healRequester) decide(staged *execute.StagingTxn, stream execute.StreamID, delivered, blockIndex uint64) [][2]uint64 {
	// The walk runs to whichever list reaches further: entries beyond the
	// validated hashes are a gap of proof, validated hashes beyond the
	// entries are a gap of entries.
	sighted := staged.Sighted(stream)
	if reach := staged.Reach(stream); reach > sighted {
		sighted = reach
	}
	r.forget(stream, delivered)
	if sighted <= delivered {
		// Nothing held above Delivered: every validating hash above it is
		// missing, so the span above Delivered is asked for whole. The source
		// answers with what it has dispatched, or that it has produced
		// nothing there yet. A lost package -- entries and proof together --
		// leaves exactly this, and nothing else would ever see it.
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

	r.mu.Lock()
	defer r.mu.Unlock()
	asks := r.asks[streamKey(stream)]
	var spans [][2]uint64
	for n := delivered + 1; n <= through; n++ {
		h, held := staged.IDOf(stream, n)
		if held && (!h.Collected || staged.IsValidated(stream, n, h.Hash) || proofWaiting(n)) {
			continue // held and validated, or its proof is waiting for its anchor: nothing to ask
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
func (c *Conductor) requestSpan(ctx context.Context, ranger private.SequenceRanger, source *url.URL, first, last uint64) (int, uint64, error) {
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
func (c *Conductor) requestAnchorSpan(ctx context.Context, ranger private.SequenceRanger, source *url.URL, first, last uint64) (int, uint64, error) {
	records, err := ranger.SequenceRange(ctx, source.JoinPath(protocol.AnchorPool), c.Url(), first, last, private.SequenceOptions{})
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

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

// gapMemory is what the requester remembers about one asked index: the block
// it asked at. Node state, not consensus state; a restart empties it at the
// cost of one duplicate request.
type gapMemory struct {
	askedAt uint64
}

type healRequester struct {
	mu       sync.Mutex
	gaps     map[string]map[uint64]*gapMemory // stream -> index
	backoff  map[string]uint64                // source -> block before which it is not asked
	failures map[string]uint
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
			// Not a gap yet, not a failure.
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
	if sighted <= delivered {
		// Nothing held above Delivered: every validating hash above it is
		// missing, so the span above Delivered is asked for whole. The source
		// answers with what it has dispatched, or that it has produced
		// nothing there yet. A lost package -- entries and proof together --
		// leaves exactly this, and nothing else would ever see it.
		r.forget(stream, delivered)
		r.mu.Lock()
		defer r.mu.Unlock()
		if g := r.gaps[streamKey(stream)][delivered+1]; g != nil && blockIndex-g.askedAt < healPatience*healCadence {
			return nil
		}
		return [][2]uint64{{delivered + 1, delivered + protocol.MaxReceiptListElements}}
	}
	through := sighted
	if through > delivered+healHorizon {
		through = delivered + healHorizon
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gaps == nil {
		r.gaps = map[string]map[uint64]*gapMemory{}
	}
	key := streamKey(stream)
	mem := r.gaps[key]
	for n := range mem {
		if n <= delivered {
			delete(mem, n)
		}
	}

	var spans [][2]uint64
	for n := delivered + 1; n <= through; n++ {
		h, held := staged.IDOf(stream, n)
		if held && (!h.Collected || staged.IsValidated(stream, n, h.Hash)) {
			continue // held and validated: runnable, nothing to ask
		}
		if g := mem[n]; g != nil && g.askedAt != 0 && blockIndex-g.askedAt < healPatience*healCadence {
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

// forget drops what a stream's memory holds at or below Delivered; what was
// asked above it keeps its asked-at.
func (r *healRequester) forget(stream execute.StreamID, delivered uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for n := range r.gaps[streamKey(stream)] {
		if n <= delivered {
			delete(r.gaps[streamKey(stream)], n)
		}
	}
}

// asked records that every index of the span was asked on this block.
func (r *healRequester) asked(stream execute.StreamID, span [2]uint64, blockIndex uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gaps == nil {
		r.gaps = map[string]map[uint64]*gapMemory{}
	}
	mem := r.gaps[streamKey(stream)]
	if mem == nil {
		mem = map[uint64]*gapMemory{}
		r.gaps[streamKey(stream)] = mem
	}
	for n := span[0]; n <= span[1]; n++ {
		mem[n] = &gapMemory{askedAt: blockIndex}
	}
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
// this partition and submits each as a BlockAnchor carrying the answering
// validator's signature: one attestation towards the anchor's quorum, the
// same thing the validator's own dispatch would have carried. The source
// may answer a prefix; the number it served through is returned.
func (c *Conductor) requestAnchorSpan(ctx context.Context, ranger private.SequenceRanger, source *url.URL, first, last uint64) (int, uint64, error) {
	records, err := ranger.SequenceRange(ctx, source.JoinPath(protocol.AnchorPool), c.Url(), first, last, private.SequenceOptions{})
	if err != nil {
		return 0, 0, errors.UnknownError.Wrap(err)
	}
	if len(records) == 0 {
		return 0, 0, errors.InvalidRecord.With("empty answer")
	}
	var served uint64
	for _, r := range records {
		if r.Sequence == nil {
			return 0, 0, errors.InvalidRecord.With("answer carries an unsequenced message")
		}
		keySig := keySignatureOf(r)
		if keySig == nil {
			return 0, 0, errors.InvalidRecord.WithFormat("answer for anchor %v→%v #%d is not signed", source, c.Url(), r.Sequence.Number)
		}
		env := &messaging.Envelope{Messages: []messaging.Message{&messaging.BlockAnchor{Anchor: r.Sequence, Signature: keySig}}}
		err := c.submit(ctx, c.Url(), env)
		if err != nil {
			return 0, 0, errors.UnknownError.WithFormat("submit anchor from %v: %w", source, err)
		}
		served = r.Sequence.Number
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
	if r.Signatures == nil {
		return nil
	}
	for _, set := range r.Signatures.Records {
		if set == nil || set.Signatures == nil {
			continue
		}
		for _, s := range set.Signatures.Records {
			sm, ok := s.Message.(*messaging.SignatureMessage)
			if !ok {
				continue
			}
			if ks, ok := sm.Signature.(protocol.KeySignature); ok {
				return ks
			}
		}
	}
	return nil
}

func partitionLabel(u *url.URL) string {
	if id, ok := protocol.ParsePartitionUrl(u); ok {
		return id
	}
	return u.ShortString()
}

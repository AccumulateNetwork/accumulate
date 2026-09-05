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

// The requesting side of healing (healing.md, "Who asks, and when" and
// "Deciding, in staging"). On an activation block a selected validator reads
// its synthetic staging: every index above Delivered that staging does not
// hold, or holds collected but unproven, is a gap. Gaps that have been visible
// for healNoticeAge activations and were not asked within healPatience
// activations are coalesced into index spans and requested from the source
// partition, whose answer — the entries and one collection proof — is
// submitted into this partition as a bundle. Nothing here reads history: the
// decision comes from staging and the synthetic ledger, the answer from the
// source's cache.

const (
	// healNoticeAge is how many activations a gap must have been visible
	// before it is asked for. Delivery is in flight for a few blocks after
	// dispatch; asking sooner asks about entries already on their way.
	healNoticeAge = 2

	// healExpectedAge is the notice age for an index the destination has not
	// sighted at all but the source's ledger says it produced. Production runs
	// ahead of dispatch by the Directory round trip, so an unsighted index is
	// in flight for longer than a hole below a sighted one.
	healExpectedAge = 6

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

// gapMemory is what the requester remembers about one missing index, in block
// indexes: when it first saw the gap and when it last asked. Node state, not
// consensus state; a restart empties it at the cost of one duplicate request.
type gapMemory struct {
	firstSeen uint64
	askedAt   uint64
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
	var ledger *protocol.SyntheticLedger
	err := batch.Account(c.Url(protocol.Synthetic)).Main().GetAs(&ledger)
	if err != nil {
		return errors.UnknownError.WithFormat("load synthetic ledger: %w", err)
	}

	staged := c.Staging.Begin()
	defer staged.Discard()

	for _, source := range c.inboundSources(ledger) {
		if c.requester.backedOff(source, blockIndex) {
			continue
		}
		stream := execute.StreamID{Ledger: c.Url(protocol.Synthetic), Source: source}
		expected := c.expectedFrom(ctx, source)
		spans := c.requester.decide(staged, stream, ledger.Partition(source).Delivered, expected, blockIndex)
		if len(spans) == 0 {
			continue
		}
		asked, failed := 0, 0
		for _, span := range spans {
			if ctx.Err() != nil {
				break
			}
			n, served, err := c.requestSpan(ctx, ranger, source, span[0], span[1])
			switch {
			case err == nil:
				asked++
				c.requester.asked(stream, [2]uint64{span[0], served}, blockIndex)
				mHealRequests.WithLabelValues("answered", c.Partition.ID, partitionLabel(source)).Inc()
				mHealEntries.Add(float64(n))
				if c.Heals != nil {
					c.Heals.Requests.Add(1)
					c.Heals.Synthetic.Add(uint64(n))
				}
				c.synthHeals.Add(uint64(n))
				slog.WarnContext(ctx, "Requested missing synthetics", "module", "conductor",
					"source", source, "destination", c.Url(), "start", span[0], "end", span[1], "entries", n, "block", blockIndex)
			case errors.Is(err, errors.NotReady):
				// The source has not dispatched the span, or dispatched it
				// within the last few blocks: the entries are on their way.
				// Not a gap yet, not a failure.
				mHealRequests.WithLabelValues("not-yet", c.Partition.ID, partitionLabel(source)).Inc()
				slog.DebugContext(ctx, "Missing synthetics are still in flight at the source", "module", "conductor",
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
				slog.ErrorContext(ctx, "Source cannot serve missing synthetics", "module", "conductor",
					"source", source, "destination", c.Url(), "start", span[0], "end", span[1], "error", err)
			default:
				failed++
				mHealRequests.WithLabelValues("failed", c.Partition.ID, partitionLabel(source)).Inc()
				if c.Heals != nil {
					c.Heals.Requests.Add(1)
				}
				slog.ErrorContext(ctx, "Failed to request missing synthetics", "module", "conductor",
					"source", source, "destination", c.Url(), "start", span[0], "end", span[1], "error", err)
			}
		}
		c.requester.outcome(source, blockIndex, asked, failed)
	}

	// Bundles were submitted to the dispatcher by this task, after the block
	// hook's own Send ran. Send them now rather than a block later.
	for err := range c.Dispatcher.Send(ctx) {
		slog.ErrorContext(ctx, "Failed to dispatch healing bundle", "module", "conductor", "error", err)
	}
	return nil
}

// expectedFrom asks the source what it has produced for this partition: its
// synthetic ledger's Produced count for us. A lost tail leaves nothing in
// staging to reveal a gap; the source's ledger is the one place the expected
// indexes are written. Mutable state at the source, one query per activation;
// a failed query expects nothing, and the tail is asked about next time.
func (c *Conductor) expectedFrom(ctx context.Context, source *url.URL) uint64 {
	var ledger *protocol.SyntheticLedger
	_, err := c.Querier.QueryAccountAs(ctx, source.JoinPath(protocol.Synthetic), nil, &ledger)
	if err != nil {
		slog.DebugContext(ctx, "Failed to query the source's synthetic ledger", "module", "conductor", "source", source, "error", err)
		return 0
	}
	for _, part := range ledger.Sequence {
		if part.Url != nil && strings.EqualFold(part.Url.String(), c.Url().String()) {
			return part.Produced
		}
	}
	return 0
}

// decide computes the spans to ask a source for: indexes in (delivered,
// max(sighted, expected)] that staging does not hold or holds unproven, first
// seen healNoticeAge activations ago (healExpectedAge above what is sighted)
// and not asked within healPatience.
func (r *healRequester) decide(staged *execute.StagingTxn, stream execute.StreamID, delivered, expected, blockIndex uint64) [][2]uint64 {
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

	sighted := staged.Sighted(stream)
	through := sighted
	if expected > through {
		through = expected
	}
	if through <= delivered {
		return nil
	}
	if through > delivered+healHorizon {
		through = delivered + healHorizon
	}
	if mem == nil {
		mem = map[uint64]*gapMemory{}
		r.gaps[key] = mem
	}

	// A collected entry whose proof has arrived and waits for its Directory
	// anchor is not a gap: the proof is staged, the anchor is on its way
	// (late when the destination's executor lags), and asking the source
	// again lands the entry twice (run 20260905T134346Z: 22,642 heals with
	// nothing dropped). Only an entry no staged proof covers is unproven.
	covered := map[[32]byte]bool{}
	for _, block := range staged.ProofBlocks(stream.Source) {
		for _, proof := range staged.Proofs(stream.Source, block) {
			if proof == nil || proof.ReceiptList == nil {
				continue
			}
			for _, e := range proof.ReceiptList.Elements {
				if len(e) == 32 {
					covered[*(*[32]byte)(e)] = true
				}
			}
		}
	}

	var spans [][2]uint64
	for n := delivered + 1; n <= through; n++ {
		h, held := staged.IDOf(stream, n)
		if held && (!h.Collected || covered[h.Hash] || staged.IsProven(stream, h.Hash)) {
			continue // held and runnable, or its proof is waiting for its anchor: nothing to ask
		}
		g := mem[n]
		if g == nil {
			mem[n] = &gapMemory{firstSeen: blockIndex}
			continue
		}
		age := uint64(healNoticeAge)
		if n > sighted {
			age = healExpectedAge
		}
		if blockIndex-g.firstSeen < age*healCadence {
			continue
		}
		if g.askedAt != 0 && blockIndex-g.askedAt < healPatience*healCadence {
			continue
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

// asked records that every index of the span was asked on this block.
func (r *healRequester) asked(stream execute.StreamID, span [2]uint64, blockIndex uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	mem := r.gaps[streamKey(stream)]
	for n := span[0]; n <= span[1]; n++ {
		if g := mem[n]; g != nil {
			g.askedAt = blockIndex
		}
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

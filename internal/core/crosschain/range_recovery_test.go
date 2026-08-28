// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// This file is the #4138 characterization suite: it pins the Conductor's
// range-recovery behaviour BEFORE the executor's in-block recovery path is
// deleted. Every test here covers the path that survives, so that "the
// Conductor covers everything the deleted path did" is a proven claim, not an
// assumption. Plan: #4134, umbrella: #4137.

// perMessageSequencer implements private.Sequencer but NOT
// private.SequenceRanger, and records which sequence numbers were requested.
type perMessageSequencer struct {
	calls []uint64
}

func (s *perMessageSequencer) Sequence(_ context.Context, _, _ *url.URL, num uint64, _ private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	s.calls = append(s.calls, num)
	return nil, errors.NotFound.With("nothing to serve")
}

// recordingRanger is a fakeRanger that also records per-message Sequence
// calls, so a test can see which of the two pull paths actually ran.
type recordingRanger struct {
	fakeRanger
	seqCalls []uint64
}

func (r *recordingRanger) Sequence(_ context.Context, _, _ *url.URL, num uint64, _ private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	r.seqCalls = append(r.seqCalls, num)
	return nil, errors.NotFound.With("nothing to serve")
}

// newRangeConductor builds a Conductor wired the way the range-recovery tests
// need: partition BVN1, Kourou active, submissions captured instead of
// dispatched.
func newRangeConductor(seq private.Sequencer, submitted *[]*messaging.Envelope) *Conductor {
	c := &Conductor{
		Partition: &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator},
		Sequencer: seq,
		Intercept: func(_ context.Context, env *messaging.Envelope) (bool, error) {
			*submitted = append(*submitted, env)
			return false, nil // capture, do not dispatch
		},
		SyntheticHealWindow: time.Minute,
	}
	c.Globals.Store(&core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV2Kourou})
	return c
}

// anchorLedgerBatch returns a writable batch over a fresh in-memory database
// holding BVN1's anchor ledger, with `delivered` anchors executed from source.
func anchorLedgerBatch(t *testing.T, source *url.URL, delivered uint64) *database.Batch {
	t.Helper()
	self := protocol.PartitionUrl("BVN1")
	ledger := new(protocol.AnchorLedger)
	ledger.Url = self.JoinPath(protocol.AnchorPool)
	ledger.Partition(source).Delivered = delivered

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))
	return batch
}

// rangeProofAnchor decides whether the range path is usable at all, and which
// held anchor bounds the proof. Callers rely on it refusing cleanly: a wrong
// "yes" produces range requests the source cannot prove (#4086), a wrong "no"
// merely falls back to per-message pulls.

func TestRangeProofAnchor_RefusesBelowKourou(t *testing.T) {
	source := protocol.PartitionUrl("BVN2")
	var submitted []*messaging.Envelope
	c := newRangeConductor(&fakeRanger{}, &submitted)
	// Everything else is valid — an anchor is held, the sequencer serves
	// ranges — but collection proofs do not exist before Kourou.
	c.Globals.Store(&core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV2Tanegashima})

	batch := anchorLedgerBatch(t, source, 42)
	held, ok := c.rangeProofAnchor(batch, source)
	assert.False(t, ok, "the range path must be refused below Kourou")
	assert.Zero(t, held)
}

func TestRangeProofAnchor_RefusesWhenSequencerIsNotARanger(t *testing.T) {
	source := protocol.PartitionUrl("BVN2")
	var submitted []*messaging.Envelope
	c := newRangeConductor(&perMessageSequencer{}, &submitted)

	batch := anchorLedgerBatch(t, source, 42)
	_, ok := c.rangeProofAnchor(batch, source)
	assert.False(t, ok, "a sequencer that does not serve ranges must refuse the range path cleanly")
}

// The BVN→BVN case: partitions hold no anchors from each other, so there is
// no root to verify a collection proof against and the range path must
// decline — this is exactly the scenario the e2e healing tests exercise.
func TestRangeProofAnchor_RefusesWhenNothingFromSourceIsAnchored(t *testing.T) {
	source := protocol.PartitionUrl("BVN2")
	var submitted []*messaging.Envelope
	c := newRangeConductor(&fakeRanger{}, &submitted)

	batch := anchorLedgerBatch(t, source, 0)
	held, ok := c.rangeProofAnchor(batch, source)
	assert.False(t, ok, "with nothing anchored from the source there is no root to prove against")
	assert.Zero(t, held)
}

func TestRangeProofAnchor_ReturnsNewestExecutedAnchor(t *testing.T) {
	source := protocol.PartitionUrl("BVN2")
	var submitted []*messaging.Envelope
	c := newRangeConductor(&fakeRanger{}, &submitted)

	batch := anchorLedgerBatch(t, source, 42)
	held, ok := c.rangeProofAnchor(batch, source)
	require.True(t, ok)
	assert.Equal(t, uint64(42), held,
		"the proof bound is the newest anchor we have EXECUTED from the source")
}

func TestRangeProofAnchor_RefusesWhenAnchorLedgerIsMissing(t *testing.T) {
	source := protocol.PartitionUrl("BVN2")
	var submitted []*messaging.Envelope
	c := newRangeConductor(&fakeRanger{}, &submitted)

	// A fresh database with no anchor ledger at all — the load fails.
	db := database.OpenInMemory(nil)
	batch := db.Begin(false)
	defer batch.Discard()

	require.NotPanics(t, func() {
		held, ok := c.rangeProofAnchor(batch, source)
		assert.False(t, ok, "a load error must refuse the range path, not panic or claim it")
		assert.Zero(t, held)
	})
}

// firstMissingRun: a known entry BEFORE the first hole is skipped, not a
// terminator — it only ends a run that has already started. Recorded in the
// #4138 test plan correction after reading the real code.

func TestFirstMissingRun_SkipsKnownEntriesBeforeTheFirstHole(t *testing.T) {
	known := protocol.PartitionUrl("BVN2").WithTxID([32]byte{1})
	part := &protocol.PartitionSyntheticLedger{
		Delivered: 5,
		Pending:   []*url.TxID{known, nil, nil},
	}
	first, last, ok := firstMissingRun(part)
	require.True(t, ok)
	assert.Equal(t, uint64(7), first, "the run starts at the first HOLE, past the known entry")
	assert.Equal(t, uint64(8), last)
}

func TestFirstMissingRun_KnownEntryEndsARunOnlyOnceStarted(t *testing.T) {
	known := protocol.PartitionUrl("BVN2").WithTxID([32]byte{1})
	part := &protocol.PartitionSyntheticLedger{
		Delivered: 5,
		Pending:   []*url.TxID{nil, known, nil},
	}
	first, last, ok := firstMissingRun(part)
	require.True(t, ok)
	assert.Equal(t, uint64(6), first)
	assert.Equal(t, uint64(6), last,
		"the known entry terminates the started run; the hole after it belongs to the next scan")
}

func TestFirstMissingRun_AllKnownReturnsFalse(t *testing.T) {
	known := protocol.PartitionUrl("BVN2").WithTxID([32]byte{1})
	part := &protocol.PartitionSyntheticLedger{
		Delivered: 5,
		Pending:   []*url.TxID{known, known, known},
	}
	_, _, ok := firstMissingRun(part)
	assert.False(t, ok, "a fully-known pending window has nothing to recover")
}

// recoverSyntheticsViaRange: the range pull itself. Clamping and proof reuse
// are pinned by TestRecoverSyntheticsViaRange_ProofBoundAndReuse; these cover
// the rest of the contract.

func TestRecoverSyntheticsViaRange_RejectsResponseCarryingNoCollectionProof(t *testing.T) {
	self := protocol.PartitionUrl("BVN1")
	source := protocol.PartitionUrl("BVN2")

	var submitted []*messaging.Envelope
	// list == nil: the served records carry no SourceReceiptList.
	ranger := &fakeRanger{src: source, dst: self}
	c := newRangeConductor(ranger, &submitted)

	done, err := c.recoverSyntheticsViaRange(context.Background(), source, 3, 5, 9)
	assert.False(t, done)
	require.Error(t, err, "a response with no collection proof is a protocol violation, not a silent skip")
	assert.ErrorContains(t, err, "carries no collection proof")
	assert.Empty(t, submitted, "nothing may be submitted without a proof")
}

func TestRecoverSyntheticsViaRange_AnchorMetadataNamesTheSourceNotTheDirectory(t *testing.T) {
	self := protocol.PartitionUrl("BVN1")
	source := protocol.PartitionUrl("BVN2")

	var submitted []*messaging.Envelope
	ranger := &fakeRanger{list: &merkle.ReceiptList{}, src: source, dst: self}
	c := newRangeConductor(ranger, &submitted)

	done, err := c.recoverSyntheticsViaRange(context.Background(), source, 3, 4, 9)
	require.NoError(t, err)
	require.True(t, done)
	require.NotEmpty(t, submitted)
	for _, env := range submitted {
		sm, ok := env.Messages[0].(*messaging.SyntheticMessage)
		require.True(t, ok)
		require.NotNil(t, sm.Proof.Anchor)
		assert.True(t, sm.Proof.Anchor.Account.Equal(source),
			"the collection proof terminates at a root of the SOURCE — the directory is out of the path entirely, that being the point of #4087")
	}
}

// companionRanger serves records whose sequenced message is built by mkMsg —
// so tests can serve MessageForTransaction types (SignatureRequest) and block
// anchors, which fakeRanger's plain transactions cannot express.
type companionRanger struct {
	src, dst *url.URL
	list     *merkle.ReceiptList
	mkMsg    func(n uint64) messaging.Message
}

func (f *companionRanger) Sequence(context.Context, *url.URL, *url.URL, uint64, private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	return nil, errors.NotFound.With("per-message path not under test")
}

func (f *companionRanger) SequenceRange(_ context.Context, _, _ *url.URL, start, end uint64, _ private.SequenceOptions) ([]*api.MessageRecord[messaging.Message], error) {
	var records []*api.MessageRecord[messaging.Message]
	for n := start; n <= end; n++ {
		msg := f.mkMsg(n)
		seq := &messaging.SequencedMessage{
			Message:     msg,
			Source:      f.src,
			Destination: f.dst,
			Number:      n,
		}
		records = append(records, &api.MessageRecord[messaging.Message]{
			Message:  msg,
			Sequence: seq,
			Signatures: &api.RecordRange[*api.SignatureSetRecord]{
				Records: []*api.SignatureSetRecord{{
					Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{
						Records: []*api.MessageRecord[messaging.Message]{{
							Message: &messaging.SignatureMessage{
								Signature: &protocol.ED25519Signature{PublicKey: make([]byte, 32)},
							},
						}},
					},
				}},
			},
		})
	}
	records[len(records)-1].SourceReceiptList = f.list
	return records, nil
}

// companionQuerier answers transaction queries with a canned transaction and
// records what was asked for, so a test can assert whether the companion was
// bundled — and from where.
type companionQuerier struct {
	asked *[]*url.URL
	txn   *protocol.Transaction
}

func (q companionQuerier) Query(_ context.Context, scope *url.URL, _ api.Query) (api.Record, error) {
	*q.asked = append(*q.asked, scope)
	return &api.MessageRecord[messaging.Message]{
		ID:      q.txn.ID(),
		Message: &messaging.TransactionMessage{Transaction: q.txn},
	}, nil
}

// The #4066 property: a synthetic message FOR a transaction (SignatureRequest,
// CreditPayment) is useless to a destination that never received the
// transaction itself — without the companion the healed message fails on
// "load transaction" and the stream stays stuck.
func TestRecoverSyntheticsViaRange_BundlesCompanionForMessageForTransaction(t *testing.T) {
	self := protocol.PartitionUrl("BVN1")
	source := protocol.PartitionUrl("BVN2")

	txn := new(protocol.Transaction)
	txn.Header.Principal = self.JoinPath("some", "account")
	txn.Body = &protocol.SendTokens{}

	ranger := &companionRanger{
		src: source, dst: self, list: &merkle.ReceiptList{},
		mkMsg: func(uint64) messaging.Message {
			return &messaging.SignatureRequest{TxID: txn.ID()}
		},
	}

	var submitted []*messaging.Envelope
	var asked []*url.URL
	c := newRangeConductor(ranger, &submitted)
	c.Querier = api.Querier2{Querier: companionQuerier{asked: &asked, txn: txn}}

	done, err := c.recoverSyntheticsViaRange(context.Background(), source, 3, 4, 9)
	require.NoError(t, err)
	require.True(t, done)
	require.Len(t, submitted, 2)
	for _, env := range submitted {
		require.Len(t, env.Messages, 2, "the companion transaction must ride in the same envelope")
		_, ok := env.Messages[1].(*messaging.TransactionMessage)
		assert.True(t, ok, "the second message is the companion transaction itself")
	}
	require.Len(t, asked, 2)
	for _, scope := range asked {
		assert.Equal(t, source.Authority, scope.Authority,
			"the companion is queried from the SOURCE — the destination provably may not have it")
	}
}

// Block anchors are MessageForTransaction too (they wrap a sequenced
// transaction), but their transaction is the anchor body the destination
// executes directly — bundling would query for a transaction that is not an
// account transaction at all.
func TestRecoverSyntheticsViaRange_DoesNotBundleCompanionForBlockAnchor(t *testing.T) {
	self := protocol.PartitionUrl("BVN1")
	source := protocol.PartitionUrl("BVN2")

	txn := new(protocol.Transaction)
	txn.Header.Principal = self.JoinPath(protocol.AnchorPool)
	txn.Body = &protocol.BlockValidatorAnchor{}

	ranger := &companionRanger{
		src: source, dst: self, list: &merkle.ReceiptList{},
		mkMsg: func(n uint64) messaging.Message {
			return &messaging.BlockAnchor{
				Anchor: &messaging.SequencedMessage{
					Message:     &messaging.TransactionMessage{Transaction: txn},
					Source:      source,
					Destination: self,
					Number:      n,
				},
				Signature: &protocol.ED25519Signature{PublicKey: make([]byte, 32)},
			}
		},
	}

	var submitted []*messaging.Envelope
	var asked []*url.URL
	c := newRangeConductor(ranger, &submitted)
	c.Querier = api.Querier2{Querier: companionQuerier{asked: &asked, txn: txn}}

	done, err := c.recoverSyntheticsViaRange(context.Background(), source, 3, 3, 9)
	require.NoError(t, err)
	require.True(t, done)
	require.Len(t, submitted, 1)
	assert.Len(t, submitted[0].Messages, 1, "a block anchor needs no companion")
	assert.Empty(t, asked, "no companion query may be issued for a block anchor")
}

func TestRecoverSyntheticsViaRange_IncrementsHealsTotalPerMessage(t *testing.T) {
	self := protocol.PartitionUrl("BVN1")
	source := protocol.PartitionUrl("BVN2")

	counter := mHeals.WithLabelValues("synthetic-range", "BVN1", "BVN2")
	before := testutil.ToFloat64(counter)

	var submitted []*messaging.Envelope
	ranger := &fakeRanger{list: &merkle.ReceiptList{}, src: source, dst: self}
	c := newRangeConductor(ranger, &submitted)
	c.Heals = new(HealCounters)

	done, err := c.recoverSyntheticsViaRange(context.Background(), source, 3, 6, 9)
	require.NoError(t, err)
	require.True(t, done)
	require.Len(t, submitted, 4)

	assert.Equal(t, float64(4), testutil.ToFloat64(counter)-before,
		"heals_total{type=synthetic-range} counts every recovered message — an invisible recovery is indistinguishable from one that never ran (#4073)")
	assert.Equal(t, uint64(4), c.Heals.Synthetic.Load(),
		"the operator-visible counter must agree")
}

func TestRecoverSyntheticsViaRange_ReturnsFalseWhenPeerDoesNotServeRanges(t *testing.T) {
	source := protocol.PartitionUrl("BVN2")

	var submitted []*messaging.Envelope
	seq := &perMessageSequencer{}
	c := newRangeConductor(seq, &submitted)

	done, err := c.recoverSyntheticsViaRange(context.Background(), source, 3, 5, 9)
	assert.False(t, done, "the caller must fall back to per-message pulls, not treat the stream as handled")
	assert.NoError(t, err, "an old peer is not an error — the fallback is the design")
	assert.Empty(t, seq.calls, "the range path itself must not fall back on the caller's behalf")
}

// The ordering property a regression would silently break: usability of the
// range path is checked BEFORE any sequence is claimed. If the fast path
// claimed first and then discovered it was unusable, the claim would suppress
// the per-message fallback until the window expired — a stream stalled for a
// full heal window with both recovery paths idle.
func TestUsabilityIsCheckedBeforeClaimingASequence(t *testing.T) {
	self := protocol.PartitionUrl("BVN1")
	source := protocol.PartitionUrl("BVN2")

	// The stream has a visible gap: delivered 5, received 9, holes at 6-8.
	synth := new(protocol.SyntheticLedger)
	synth.Url = self.JoinPath(protocol.Synthetic)
	sp := synth.Partition(source)
	sp.Delivered = 5
	sp.Received = 9
	sp.Pending = []*url.TxID{nil, nil, nil, source.WithTxID([32]byte{9})}

	// The range path is UNUSABLE: nothing from BVN2 has ever been anchored
	// into us (the BVN→BVN case), so rangeProofAnchor refuses.
	anchors := new(protocol.AnchorLedger)
	anchors.Url = self.JoinPath(protocol.AnchorPool)
	anchors.Partition(source).Delivered = 0

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()
	require.NoError(t, batch.Account(synth.Url).Main().Put(synth))
	require.NoError(t, batch.Account(anchors.Url).Main().Put(anchors))

	ranger := &recordingRanger{}
	var submitted []*messaging.Envelope
	c := newRangeConductor(ranger, &submitted)

	// Seed the per-stream throttle as already fired, so the scan acts now.
	c.synthHealState = map[string]*synthHealEntry{
		source.String(): {want: 6, fireAt: time.Now().Add(-time.Hour)},
	}

	require.NoError(t, c.requestMissingSynthetics(context.Background(), batch))

	assert.Empty(t, ranger.calls, "the unusable range path must not issue a range request")
	assert.Subset(t, ranger.seqCalls, []uint64{6, 7, 8},
		"the per-message fallback must pull every hole in the SAME scan — not sit suppressed behind a claim the dead fast path made")
}

// The anchor-side range recovery names no proof bound: anchors prove against
// the source's own root, and the source infers the bound as end+1 — the
// anchor whose arrival exposed the hole.
func TestRecoverAnchorsViaRange_SendsBareSequenceOptions(t *testing.T) {
	self := protocol.PartitionUrl("BVN1")
	source := protocol.DnUrl()

	held := source.WithTxID([32]byte{8})
	ledger := new(protocol.AnchorLedger)
	ledger.Url = self.JoinPath(protocol.AnchorPool)
	lp := ledger.Partition(source)
	lp.Delivered = 5
	lp.Pending = []*url.TxID{nil, nil, held}

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))

	ranger := &fakeRanger{list: &merkle.ReceiptList{}, src: source, dst: self}
	var submitted []*messaging.Envelope
	c := newRangeConductor(ranger, &submitted)
	c.synthHealState = map[string]*synthHealEntry{
		source.JoinPath("anchor-range").String(): {want: 6, fireAt: time.Now().Add(-time.Hour)},
	}

	require.NoError(t, c.recoverAnchorsViaRange(context.Background(), batch, source))

	require.Len(t, ranger.opts, 1)
	assert.Equal(t, private.SequenceOptions{}, ranger.opts[0],
		"the anchor range request carries bare options — the source infers the proof bound as end+1")
}

func TestRecoverAnchorsViaRange_IncrementsHealsTotalWithAnchorRangeLabel(t *testing.T) {
	self := protocol.PartitionUrl("BVN1")
	source := protocol.DnUrl()

	counter := mHeals.WithLabelValues("anchor-range", "BVN1", protocol.Directory)
	before := testutil.ToFloat64(counter)

	held := source.WithTxID([32]byte{8})
	ledger := new(protocol.AnchorLedger)
	ledger.Url = self.JoinPath(protocol.AnchorPool)
	lp := ledger.Partition(source)
	lp.Delivered = 5
	lp.Pending = []*url.TxID{nil, nil, held}

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))

	ranger := &fakeRanger{list: &merkle.ReceiptList{}, src: source, dst: self}
	var submitted []*messaging.Envelope
	c := newRangeConductor(ranger, &submitted)
	c.Heals = new(HealCounters)
	c.synthHealState = map[string]*synthHealEntry{
		source.JoinPath("anchor-range").String(): {want: 6, fireAt: time.Now().Add(-time.Hour)},
	}

	require.NoError(t, c.recoverAnchorsViaRange(context.Background(), batch, source))
	require.Len(t, submitted, 2)

	assert.Equal(t, float64(2), testutil.ToFloat64(counter)-before,
		"heals_total{type=anchor-range} counts every recovered anchor")
	assert.Equal(t, uint64(2), c.Heals.Anchor.Load())
}

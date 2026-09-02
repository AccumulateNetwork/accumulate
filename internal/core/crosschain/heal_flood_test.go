// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// These tests pin the healing economics: recovery must not flood the network
// while it fixes a stream, requests must cover only what is actually missing,
// and a collection proof — once served — is carried through as-is, never
// rebuilt per message. The 20260819T234054Z collapse and the 20260820 soak
// aborts were all, at bottom, healing machinery doing more work than the gap
// it was closing.

// TestClaimSyntheticRequest_NoFlood pins the request throttle: on first sight
// of a gap a node schedules a jittered fire time and does NOT request; it
// fires only after the jitter elapses; and consecutive requests for the same
// gap are separated by at least the back-off window. Every validator runs
// this logic every block — without the throttle, twelve validators would
// hammer the source with the same request dozens of times per second.
func TestClaimSyntheticRequest_NoFlood(t *testing.T) {
	c := new(Conductor)
	c.SyntheticHealWindow = time.Minute
	src := protocol.PartitionUrl("BVN2")
	t0 := time.Now()

	// First sight schedules; it must not fire immediately.
	require.False(t, c.claimSyntheticRequest(src, 7, t0),
		"first sight of a gap schedules a jittered request, it must not fire")

	// Within the window the claim may or may not fire depending on the
	// jitter; after a full window it must fire.
	require.True(t, c.claimSyntheticRequest(src, 7, t0.Add(time.Minute+time.Millisecond)),
		"after the full jitter window the request must fire")

	// Immediately after firing: back-off. No second request inside the window.
	require.False(t, c.claimSyntheticRequest(src, 7, t0.Add(time.Minute+2*time.Millisecond)),
		"a request immediately after another is a flood")
	require.False(t, c.claimSyntheticRequest(src, 7, t0.Add(90*time.Second)),
		"still inside the back-off window")

	// After the back-off elapses the gap may be retried.
	require.True(t, c.claimSyntheticRequest(src, 7, t0.Add(2*time.Minute+2*time.Millisecond)),
		"after the back-off the retry is allowed")

	// The gap advancing (someone healed part of it) starts a fresh jitter —
	// again no immediate fire.
	require.False(t, c.claimSyntheticRequest(src, 9, t0.Add(2*time.Minute+3*time.Millisecond)),
		"an advanced gap is a new gap: schedule, do not fire")
}

// TestRunExclusive_ScansDoNotStack: healing scans are scheduled from every
// block, and a scan that outlives the block interval must not stack a second
// copy of itself — copies used to stack without bound (#4115).
func TestRunExclusive_ScansDoNotStack(t *testing.T) {
	c := new(Conductor)

	var running atomic.Int32
	var peak atomic.Int32
	release := make(chan struct{})
	done := make(chan struct{}, 3)

	task := func() {
		n := running.Add(1)
		if n > peak.Load() {
			peak.Store(n)
		}
		<-release
		running.Add(-1)
		done <- struct{}{}
	}

	c.runExclusive("scan:test", task)
	c.runExclusive("scan:test", task) // must be dropped — first still running
	c.runExclusive("scan:test", task) // must be dropped
	close(release)
	<-done

	require.Equal(t, int32(1), peak.Load(), "a scan must never stack on a running copy of itself")

	// Once the first completes, the key is free again.
	release2 := make(chan struct{})
	close(release2)
	ran := make(chan struct{}, 1)
	require.Eventually(t, func() bool {
		c.runExclusive("scan:test", func() { ran <- struct{}{} })
		select {
		case <-ran:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond, "the key must be released after the task completes")
}

// TestFirstMissingRun_RequestsOnlyMissing pins the range computation: the run
// requested from the source must cover ONLY the first contiguous hole — never
// entries already delivered, never entries already held. Requesting held
// entries would both waste the source's work and break the proof bound (the
// held entry is what proves the hole).
func TestFirstMissingRun_RequestsOnlyMissing(t *testing.T) {
	// Delivered through 5; 6 and 7 missing; 8 held.
	c, batch, part := gapFixture(t, 5, 8)
	first, last, ok := c.firstMissingRun(batch, part)
	require.True(t, ok)
	require.Equal(t, uint64(6), first)
	require.Equal(t, uint64(7), last, "the run must stop at the held entry — request only the first contiguous hole")

	// Nothing missing: nothing requested.
	c, batch, part = gapFixture(t, 5, 6)
	_, _, ok = c.firstMissingRun(batch, part)
	require.False(t, ok, "a stream with no holes must request nothing")

	// Nothing staged: nothing visible to request (the reconcile pull handles
	// invisible tail loss separately).
	c, batch, part = gapFixture(t, 5)
	_, _, ok = c.firstMissingRun(batch, part)
	require.False(t, ok)
}

// signSequenced produces a real key signature over a sequenced message, the
// way the sequencer service does, so buildSyntheticSubmission's verification
// passes.
func signSequenced(key ed25519.PrivateKey, seq *messaging.SequencedMessage) protocol.KeySignature {
	h := seq.Hash()
	sig, err := new(signing.Builder).
		SetType(protocol.SignatureTypeED25519).
		SetPrivateKey(key).
		SetUrl(protocol.DnUrl().JoinPath(protocol.Network)).
		SetVersion(1).
		SetTimestampToNow().
		Sign(h[:])
	if err != nil {
		panic(err)
	}
	return sig.(protocol.KeySignature)
}

// fakeRanger implements private.SequenceRanger, recording the ranges
// requested and serving records that carry ONE collection proof on the last
// record, exactly like the real sequencer service.
type fakeRanger struct {
	calls []([2]uint64)
	opts  []private.SequenceOptions
	list  *merkle.ReceiptList
	src   *url.URL
	dst   *url.URL
	key   ed25519.PrivateKey
}

func (f *fakeRanger) Sequence(ctx context.Context, src, dst *url.URL, num uint64, opts private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	return nil, errors.NotFound.With("per-message path not under test")
}

func (f *fakeRanger) SequenceRange(ctx context.Context, src, dst *url.URL, start, end uint64, opts private.SequenceOptions) ([]*api.MessageRecord[messaging.Message], error) {
	f.calls = append(f.calls, [2]uint64{start, end})
	f.opts = append(f.opts, opts)

	var records []*api.MessageRecord[messaging.Message]
	for n := start; n <= end; n++ {
		txn := new(protocol.Transaction)
		txn.Header.Principal = f.dst.JoinPath(protocol.AnchorPool)
		txn.Body = &protocol.BlockValidatorAnchor{}
		seq := &messaging.SequencedMessage{
			Message:     &messaging.TransactionMessage{Transaction: txn},
			Source:      f.src,
			Destination: f.dst,
			Number:      n,
		}
		var sig protocol.Signature = &protocol.ED25519Signature{PublicKey: make([]byte, 32)}
		if f.key != nil {
			sig = signSequenced(f.key, seq)
		}
		r := &api.MessageRecord[messaging.Message]{
			Message:  &messaging.TransactionMessage{Transaction: txn},
			Sequence: seq,
			Signatures: &api.RecordRange[*api.SignatureSetRecord]{
				Records: []*api.SignatureSetRecord{{
					Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{
						Records: []*api.MessageRecord[messaging.Message]{{
							Message: &messaging.SignatureMessage{Signature: sig},
						}},
					},
				}},
			},
		}
		records = append(records, r)
	}
	// One proof for the whole run, on the last record — the collection proof.
	records[len(records)-1].SourceReceiptList = f.list
	return records, nil
}

// TestRecoverAnchorsViaRange_OnlyMissing_OneProofReused pins three properties
// of the collection-proof recovery path at once:
//  1. It heals missing anchors: every anchor in the hole is resubmitted.
//  2. It requests ONLY the missing run — not delivered entries, not the held
//     entry that bounds the hole.
//  3. The served collection proof is carried through AS-IS on every recovered
//     message — one proof for the run, never rebuilt per message. Once we
//     have a collection proof there is nothing left to prove.
func TestRecoverAnchorsViaRange_OnlyMissing_OneProofReused(t *testing.T) {
	self := protocol.PartitionUrl("BVN1")
	source := protocol.DnUrl()

	// Our anchor ledger: delivered through 5 from the source, 6-7 missing,
	// 8 held (its arrival is what exposed the hole).
	ledger := new(protocol.AnchorLedger)
	ledger.Url = self.JoinPath(protocol.AnchorPool)
	lp := ledger.Partition(source)
	lp.Delivered = 5

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))

	ranger := &fakeRanger{list: &merkle.ReceiptList{}, src: source, dst: self}

	var submitted []*messaging.Envelope
	c := &Conductor{
		Partition: &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator},
		Sequencer: ranger,
		Intercept: func(_ context.Context, env *messaging.Envelope) (bool, error) {
			submitted = append(submitted, env)
			return false, nil // capture, do not dispatch
		},
		SyntheticHealWindow: time.Minute,
	}
	c.Globals.Store(&core.GlobalValues{
		ExecutorVersion: protocol.ExecutorVersionV2Kourou,
	})
	stageHeldAnchors(t, batch, c, source, 8) // 6 and 7 are the hole; 8 is what exposes it

	// Seed the request throttle as already-fired so the pull happens now;
	// the throttle itself is covered by TestClaimSyntheticRequest_NoFlood.
	c.synthHealState = map[string]*synthHealEntry{
		source.JoinPath("anchor-range").String(): {want: 6, fireAt: time.Now().Add(-time.Hour)},
	}

	require.NoError(t, c.recoverAnchorsViaRange(context.Background(), batch, source))

	// 2. Exactly one request, covering exactly the missing run.
	require.Equal(t, []([2]uint64){{6, 7}}, ranger.calls,
		"the recovery must request exactly the missing run [6,7] — nothing delivered, nothing held")

	// 1. Both missing anchors were resubmitted into this partition.
	require.Len(t, submitted, 2, "every anchor in the hole must be healed")
	var seqs []uint64
	for _, env := range submitted {
		require.Len(t, env.Messages, 1)
		ba, ok := env.Messages[0].(*messaging.BlockAnchor)
		require.True(t, ok, "recovered anchors are submitted as block anchors")
		seq, ok := ba.Anchor.(*messaging.SequencedMessage)
		require.True(t, ok)
		seqs = append(seqs, seq.Number)

		// 3. The proof is the served collection proof, shared by every
		// message in the run — not rebuilt.
		require.NotNil(t, ba.Proof)
		require.Same(t, ranger.list, ba.Proof.ReceiptList,
			"the served collection proof must be carried through as-is, never rebuilt")
	}
	require.Equal(t, []uint64{6, 7}, seqs)

	// And all recovered messages share the SAME proof object.
	p0 := submitted[0].Messages[0].(*messaging.BlockAnchor).Proof
	p1 := submitted[1].Messages[0].(*messaging.BlockAnchor).Proof
	require.Same(t, p0, p1, "one proof covers the whole run")
}

// fakeQuerier serves the destination's anchor ledger to healAnchors and
// answers NotFound for everything else (so didSign sees "not signed yet").
type fakeQuerier struct {
	ledger *protocol.AnchorLedger
}

func (f fakeQuerier) Query(_ context.Context, scope *url.URL, _ api.Query) (api.Record, error) {
	// Transaction queries (didSign) carry the hash as the URL's user info;
	// account queries do not.
	if f.ledger != nil && scope.UserInfo == "" && scope.PathEqual(protocol.AnchorPool) {
		return &api.AccountRecord{Account: f.ledger}, nil
	}
	return nil, errors.NotFound.WithFormat("%v not found", scope)
}

// TestHealAnchors_HealsMissing_CappedPerScan pins two properties of the
// source-side anchor push at once:
//  1. It heals: a destination whose Delivered is genuinely stalled gets the
//     stuck anchors re-signed and resubmitted.
//  2. It does not flood: re-drives are capped per scan. Re-driving the whole
//     backlog every scan was one of the storm engines of the 20260819T234054Z
//     collapse — each scan issued one query plus one submission per missing
//     anchor, with the backlog in the thousands (#4115).
func TestHealAnchors_HealsMissing_CappedPerScan(t *testing.T) {
	self := protocol.PartitionUrl("BVN1")
	dest := protocol.DnUrl()
	const produced = 40

	// Our side: 40 anchors produced and recorded on the sequence chain.
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	record := batch.Account(self.JoinPath(protocol.AnchorPool)).AnchorSequenceChain()
	chain, err := record.Get()
	require.NoError(t, err)
	for i := 1; i <= produced; i++ {
		txn := new(protocol.Transaction)
		txn.Header.Principal = dest.JoinPath(protocol.AnchorPool)
		txn.Body = &protocol.BlockValidatorAnchor{
			PartitionAnchor: protocol.PartitionAnchor{
				Source:          self,
				MinorBlockIndex: uint64(i), // old enough to heal at currentBlock=100
			},
		}
		msg := &messaging.TransactionMessage{Transaction: txn}
		require.NoError(t, batch.Message(txn.ID().Hash()).Main().Put(msg))
		require.NoError(t, chain.AddEntry(txn.GetHash(), false))
	}

	// The destination: nothing delivered, and pinned across the stall window.
	destLedger := new(protocol.AnchorLedger)
	destLedger.Url = dest.JoinPath(protocol.AnchorPool)
	destLedger.Partition(self).Delivered = 0

	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	var submitted []*messaging.Envelope
	c := &Conductor{
		Partition:    &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator},
		ValidatorKey: key,
		Querier:      api.Querier2{Querier: fakeQuerier{ledger: destLedger}},
		Intercept: func(_ context.Context, env *messaging.Envelope) (bool, error) {
			submitted = append(submitted, env)
			return false, nil
		},
	}
	c.Globals.Store(&core.GlobalValues{
		ExecutorVersion: protocol.ExecutorVersionV2Kourou,
		Network:         &protocol.NetworkDefinition{Version: 1},
	})

	// Pin the stall window: Delivered has not advanced across StallScans.
	// (With delivered=0 every scan takes the stuck branch, so healAnchors's
	// own scan below is the one that crosses the threshold.)
	for i := 0; i < StallScans-1; i++ {
		require.False(t, c.deliveryStalled(dest.String(), 0, produced))
	}

	require.NoError(t, c.healAnchors(context.Background(), batch, dest, 100))

	// Healing happened — and stopped at the cap, not the backlog size.
	require.Len(t, submitted, 16,
		"the scan must re-drive the stuck head up to the cap (16), not the whole 40-anchor backlog")

	// The re-driven anchors are the sequential stuck head, as signed
	// block-anchor envelopes addressed to the destination.
	for i, env := range submitted {
		require.Len(t, env.Messages, 1)
		ba, ok := env.Messages[0].(*messaging.BlockAnchor)
		require.True(t, ok)
		seq, ok := ba.Anchor.(*messaging.SequencedMessage)
		require.True(t, ok)
		require.Equal(t, uint64(i+1), seq.Number, "healing must re-drive the stuck head in sequence order")
		require.True(t, seq.Destination.Equal(dest))
		require.NotNil(t, ba.Signature, "re-driven anchors carry this validator's signature")
	}
}

// TestRecoverSyntheticsViaRange_ProofBoundAndReuse pins the synthetic side of
// collection-proof recovery:
//  1. The request is proven against the anchor we already HOLD — recovery
//     never asks the source to prove above what has been anchored (#4086).
//  2. The range is capped at MaxReceiptListElements per request.
//  3. The served collection proof is shared by every recovered message.
func TestRecoverSyntheticsViaRange_ProofBoundAndReuse(t *testing.T) {
	self := protocol.PartitionUrl("BVN1")
	source := protocol.PartitionUrl("BVN2")

	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	ranger := &fakeRanger{list: &merkle.ReceiptList{}, src: source, dst: self, key: key}

	var submitted []*messaging.Envelope
	c := &Conductor{
		Partition: &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator},
		Sequencer: ranger,
		Intercept: func(_ context.Context, env *messaging.Envelope) (bool, error) {
			submitted = append(submitted, env)
			return false, nil
		},
	}
	c.Globals.Store(&core.GlobalValues{
		ExecutorVersion: protocol.ExecutorVersionV2Kourou,
	})

	const first, held = 3, 9
	// Ask for far more than one collection proof can carry.
	requestedLast := uint64(first + protocol.MaxReceiptListElements + 50)

	done, err2 := c.recoverSyntheticsViaRange(context.Background(), source, first, requestedLast, held)
	require.NoError(t, err2)
	require.True(t, done)

	// 2. One request, capped at the proof's capacity.
	require.Len(t, ranger.calls, 1)
	require.Equal(t, uint64(first), ranger.calls[0][0])
	require.Equal(t, uint64(first+protocol.MaxReceiptListElements-1), ranger.calls[0][1],
		"a single request must not exceed what one collection proof can carry")

	// 1. Proven against the anchor we hold — never above it.
	require.Equal(t, uint64(held), ranger.opts[0].ProveAgainstAnchor,
		"recovery must prove against the held anchor, not ask the source for a new proof bound")

	// 3. Every recovered message carries the served collection proof, as-is.
	require.NotEmpty(t, submitted)
	var shared *protocol.AnnotatedReceipt
	for _, env := range submitted {
		sm, ok := env.Messages[0].(*messaging.SyntheticMessage)
		require.True(t, ok)
		require.NotNil(t, sm.Proof)
		require.Same(t, ranger.list, sm.Proof.ReceiptList,
			"the served collection proof must be carried through, never rebuilt")
		if shared == nil {
			shared = sm.Proof
		} else {
			require.Same(t, shared, sm.Proof, "one proof covers the whole run")
		}
	}
}

// TestBuildSyntheticSubmission_CollectionProofWins: when a collection proof
// is in hand, it is used even if the record ALSO carries a per-message
// receipt — once we have a collection proof there is nothing left to prove,
// and building a fresh per-message proof would be pure waste. The per-message
// receipt is only the fallback when no collection proof exists.
func TestBuildSyntheticSubmission_CollectionProofWins(t *testing.T) {
	self := protocol.PartitionUrl("BVN1")
	source := protocol.PartitionUrl("BVN2")

	c := &Conductor{Partition: &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator}}
	c.Globals.Store(&core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV2Kourou})

	txn := new(protocol.Transaction)
	txn.Header.Principal = self.JoinPath("some", "account")
	txn.Body = &protocol.SyntheticDepositCredits{}
	seq := &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: txn},
		Source:      source,
		Destination: self,
		Number:      7,
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	perMessage := &merkle.Receipt{Start: []byte{1}, Anchor: []byte{2}}
	record := &api.MessageRecord[messaging.Message]{
		Sequence:      seq,
		SourceReceipt: perMessage, // present, but must NOT be used
		Signatures: &api.RecordRange[*api.SignatureSetRecord]{
			Records: []*api.SignatureSetRecord{{
				Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{
					Records: []*api.MessageRecord[messaging.Message]{{
						Message: &messaging.SignatureMessage{Signature: signSequenced(key, seq)},
					}},
				},
			}},
		},
	}

	collection := &protocol.AnnotatedReceipt{ReceiptList: &merkle.ReceiptList{}}
	env, err := c.buildSyntheticSubmission(record, collection)
	require.NoError(t, err)
	require.NotNil(t, env)
	sm, ok := env.Messages[0].(*messaging.SyntheticMessage)
	require.True(t, ok)
	require.Same(t, collection, sm.Proof,
		"with a collection proof in hand no new proof may be built — the per-message receipt must be ignored")

	// Fallback: without a collection proof, the per-message receipt is used.
	env, err = c.buildSyntheticSubmission(record, nil)
	require.NoError(t, err)
	require.NotNil(t, env)
	sm = env.Messages[0].(*messaging.SyntheticMessage)
	require.Same(t, perMessage, sm.Proof.Receipt, "without a collection proof the per-message receipt is the proof")
}

// TestBuildSyntheticSubmission_BadSignatureDoesNotBlockProvenMessages: once
// we have a collection proof no other signature is required — the proof binds
// the message hash and hashes cannot be forged, so it does not matter where
// the message came from. A bad signature must not block a proven message
// (refusing would wedge recovery after validator churn); without a collection
// proof the signature is still the authorization and a bad one is refused.
func TestBuildSyntheticSubmission_BadSignatureDoesNotBlockProvenMessages(t *testing.T) {
	self := protocol.PartitionUrl("BVN1")
	source := protocol.PartitionUrl("BVN2")

	c := &Conductor{Partition: &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator}}
	c.Globals.Store(&core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV2Kourou})

	txn := new(protocol.Transaction)
	txn.Header.Principal = self.JoinPath("some", "account")
	txn.Body = &protocol.SyntheticDepositCredits{}
	seq := &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: txn},
		Source:      source,
		Destination: self,
		Number:      7,
	}
	badSig := &protocol.ED25519Signature{
		PublicKey: make([]byte, 32),
		Signature: []byte("garbage"),
	}
	record := &api.MessageRecord[messaging.Message]{
		Sequence:      seq,
		SourceReceipt: &merkle.Receipt{Start: []byte{1}, Anchor: []byte{2}},
		Signatures: &api.RecordRange[*api.SignatureSetRecord]{
			Records: []*api.SignatureSetRecord{{
				Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{
					Records: []*api.MessageRecord[messaging.Message]{{
						Message: &messaging.SignatureMessage{Signature: badSig},
					}},
				},
			}},
		},
	}

	// With a collection proof the bad signature must not block submission.
	collection := &protocol.AnnotatedReceipt{ReceiptList: &merkle.ReceiptList{}}
	env, err := c.buildSyntheticSubmission(record, collection)
	require.NoError(t, err, "a proven message must be submitted regardless of its signature")
	require.NotNil(t, env)
	require.Same(t, collection, env.Messages[0].(*messaging.SyntheticMessage).Proof)

	// Without one, the per-message path still refuses to submit garbage.
	_, err = c.buildSyntheticSubmission(record, nil)
	require.ErrorContains(t, err, "signature is not valid")
}

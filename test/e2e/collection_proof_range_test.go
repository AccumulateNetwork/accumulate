// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestRangeRecovery drops a run of consecutive synthetic deposits between two
// partitions and verifies the destination recovers the whole run.
//
// The deposits are BVN→BVN, and for that stream the range path is correctly
// UNUSABLE today: partitions hold no anchors from each other, so the
// destination has no root to verify a collection proof against, and
// rangeProofAnchor refuses rather than asking the source to prove against a
// directory continuation — the #4086 failure the executor's deleted in-block
// path reproduced (#4138). Recovery therefore goes through the per-message
// pull, with no collection proofs on the wire, at every version. When #4140
// gives synthetics a receiver-side replica of the source's root, the range
// path becomes usable BVN→BVN and the activated case must flip back to
// expecting `recovered >= drops`.
func TestRangeRecovery(t *testing.T) {
	Run(t, map[string]ExecutorVersion{
		"activated": ExecutorVersionLatest,
		"fallback":  ExecutorVersionV2Tanegashima,
	}, func(t *testing.T, version ExecutorVersion) {
		var timestamp uint64
		const transfers = 5
		const drops = 3

		// dropped counts synthetic-deposit envelopes deliberately dropped.
		// recovered counts synthetic messages that carry a collection proof
		// (ReceiptList) — only the range-recovery path produces those.
		var dropped, recovered atomic.Int32

		globals := new(core.GlobalValues)
		globals.ExecutorVersion = version
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 3, 1),
			simulator.GenesisWith(GenesisTime, globals),

			simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
				messages, err := env.Normalize()
				if err != nil {
					return false, err
				}

				for _, msg := range messages {
					// Recovered messages are counted, never dropped.
					if syn, ok := msg.(*messaging.SyntheticMessage); ok &&
						syn.Proof != nil && syn.Proof.ReceiptList != nil {
						recovered.Add(1)
						return true, nil
					}
				}

				if dropped.Load() >= drops {
					return true, nil
				}
				for _, msg := range messages {
				again:
					switch m := msg.(type) {
					case interface{ Unwrap() messaging.Message }:
						msg = m.Unwrap()
						goto again
					case messaging.MessageWithTransaction:
						if m.GetTransaction().Body.Type() == TransactionTypeSyntheticDepositTokens {
							dropped.Add(1)
							return false, nil
						}
					}
				}
				return true, nil
			}),
		)

		// Alice and Bob must live on different partitions so the deposits are
		// cross-partition synthetic messages.
		alice := acctesting.GenerateKey("Alice")
		aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
		alicePart, err := sim.Router().RouteAccount(aliceUrl)
		require.NoError(t, err)
		var bobUrl *url.URL
		for i := 0; ; i++ {
			bob := acctesting.GenerateKey("Bob", i)
			bobUrl = acctesting.AcmeLiteAddressStdPriv(bob)
			bobPart, err := sim.Router().RouteAccount(bobUrl)
			require.NoError(t, err)
			if alicePart != bobPart {
				break
			}
		}
		MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

		// Execute. The first `drops` deposits are dropped, creating a run of
		// consecutive missing sequence numbers at Bob's partition.
		st := make([]*protocol.TransactionStatus, transfers)
		for i := range st {
			st[i] = sim.SubmitTxnSuccessfully(MustBuild(t,
				build.Transaction().For(aliceUrl).
					SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
					SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
		}
		sim.StepUntil(True(func(*Harness) bool { return dropped.Load() >= drops }))

		// Healing must recover the dropped run — every deposit executes.
		for _, st := range st {
			sim.StepUntilN(200,
				Txn(st.TxID).Succeeds(),
				Txn(st.TxID).Produced().Succeeds())
		}

		// Verify every token arrived exactly once
		lta := GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(bobUrl), bobUrl)
		require.Equal(t, transfers*int(protocol.AcmePrecision), int(lta.Balance.Uint64()))

		// No collection proofs on the wire in either case: pre-activation they
		// do not exist, and post-activation the BVN→BVN range path correctly
		// refuses because no anchor from the source is held — recovery falls
		// back to per-message pulls rather than asking for a proof the
		// destination could not check (#4138). #4140 changes this: once the
		// receiver holds a replica of the source's root, the activated case
		// must expect `recovered >= drops` again.
		require.Zero(t, recovered.Load(),
			"BVN→BVN recovery must not put collection proofs on the wire until #4140 lands")
	})
}

// TestAnchorRangeRecovery drops every copy of the directory's first anchor
// and verifies the destinations recover it with a proof-authorized anchor
// (#4056) — no signature quorum is re-gathered. The network runs THREE
// validators per partition, which signature-quorum anchor healing cannot
// handle (see TestMissingDirectoryAnchorTxn's single-validator restriction):
// quorum healing needs 2f+1 validators to each independently re-sign and
// resubmit the missed anchor, while a collection proof only needs the current
// directory root, which every synced node already has.
func TestAnchorRangeRecovery(t *testing.T) {
	var timestamp uint64

	// dropped counts copies of anchor #1 that were dropped. recovered counts
	// proof-authorized anchors — only the range-recovery path produces those.
	var dropped, recovered atomic.Int32

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 3),
		simulator.GenesisWith(GenesisTime, globals),

		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
			messages, err := env.Normalize()
			if err != nil {
				return false, err
			}

			var drop bool
			for _, msg := range messages {
				anchor, ok := msg.(*messaging.BlockAnchor)
				if !ok {
					continue
				}
				if anchor.Proof != nil {
					// A proof-authorized recovery — count it, never drop it
					recovered.Add(1)
					continue
				}
				seq, ok := anchor.Anchor.(*messaging.SequencedMessage)
				if !ok {
					continue
				}
				txn, ok := seq.Message.(*messaging.TransactionMessage)
				if !ok {
					continue
				}
				// Drop every copy of the directory's first anchor, from every
				// validator to every destination
				if txn.Transaction.Body.Type() == TransactionTypeDirectoryAnchor && seq.Number == 1 {
					dropped.Add(1)
					drop = true
				}
			}
			return !drop, nil
		}),
	)

	// Alice and Bob must live on different partitions so the deposit needs
	// cross-partition anchoring to complete.
	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	alicePart, err := sim.Router().RouteAccount(aliceUrl)
	require.NoError(t, err)
	var bobUrl *url.URL
	for i := 0; ; i++ {
		bob := acctesting.GenerateKey("Bob", i)
		bobUrl = acctesting.AcmeLiteAddressStdPriv(bob)
		bobPart, err := sim.Router().RouteAccount(bobUrl)
		require.NoError(t, err)
		if alicePart != bobPart {
			break
		}
	}
	MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

	// Execute a cross-partition transfer. Its deposit cannot be delivered
	// until the destination has the directory anchors covering it, so this
	// completes only if the dropped anchor is recovered.
	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(aliceUrl).
			SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
			SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))

	sim.StepUntil(True(func(*Harness) bool { return dropped.Load() > 0 }))

	sim.StepUntilN(400,
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// The token arrived and at least one anchor was recovered by proof
	lta := GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(bobUrl), bobUrl)
	require.Equal(t, int(protocol.AcmePrecision), int(lta.Balance.Uint64()))
	require.Greater(t, int(recovered.Load()), 0,
		"expected the dropped anchor to be recovered with a collection proof")
}

// TestAnchorQuorumStuckRecovery covers the KNOWN-but-stuck anchor case: the
// destination receives anchor #1 from exactly one validator — below the 2-of-3
// signature threshold — and every further proof-less copy is dropped, so the
// quorum can never complete (the analogue of validator churn making historical
// re-signing impossible). The anchor is a known pending entry, not an unknown
// gap, so recovery depends on healNeeded treating any pending anchor as
// healable and on the proof-authorized resubmission executing without a
// quorum (#4056).
func TestAnchorQuorumStuckRecovery(t *testing.T) {
	var timestamp uint64

	// dropped counts proof-less copies suppressed after the first; recovered
	// counts proof-authorized anchors — only the range-recovery path produces
	// those.
	var dropped, recovered atomic.Int32
	var mu sync.Mutex
	passed := map[string]bool{}

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 3),
		simulator.GenesisWith(GenesisTime, globals),

		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
			messages, err := env.Normalize()
			if err != nil {
				return false, err
			}

			var drop bool
			for _, msg := range messages {
				anchor, ok := msg.(*messaging.BlockAnchor)
				if !ok {
					continue
				}
				if anchor.Proof != nil {
					// A proof-authorized recovery — count it, never drop it
					recovered.Add(1)
					continue
				}
				seq, ok := anchor.Anchor.(*messaging.SequencedMessage)
				if !ok {
					continue
				}
				txn, ok := seq.Message.(*messaging.TransactionMessage)
				if !ok {
					continue
				}
				if txn.Transaction.Body.Type() != TransactionTypeDirectoryAnchor || seq.Number != 1 {
					continue
				}
				// Let the FIRST copy of the directory's anchor #1 through to
				// each destination — the anchor becomes a KNOWN pending entry
				// with one signature, below the 2-of-3 threshold — and drop
				// every later proof-less copy, so the quorum never completes.
				mu.Lock()
				first := !passed[seq.Destination.String()]
				passed[seq.Destination.String()] = true
				mu.Unlock()
				if !first {
					dropped.Add(1)
					drop = true
				}
			}
			return !drop, nil
		}),
	)

	// Alice and Bob must live on different partitions so the deposit needs
	// cross-partition anchoring to complete.
	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	alicePart, err := sim.Router().RouteAccount(aliceUrl)
	require.NoError(t, err)
	var bobUrl *url.URL
	for i := 0; ; i++ {
		bob := acctesting.GenerateKey("Bob", i)
		bobUrl = acctesting.AcmeLiteAddressStdPriv(bob)
		bobPart, err := sim.Router().RouteAccount(bobUrl)
		require.NoError(t, err)
		if alicePart != bobPart {
			break
		}
	}
	MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

	// Execute a cross-partition transfer. Its deposit cannot be delivered
	// until the destination has the directory anchors covering it, so this
	// completes only if the quorum-stuck anchor is recovered.
	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(aliceUrl).
			SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
			SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))

	sim.StepUntil(True(func(*Harness) bool { return dropped.Load() > 0 }))

	sim.StepUntilN(400,
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// The token arrived and at least one anchor was recovered by proof
	lta := GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(bobUrl), bobUrl)
	require.Equal(t, int(protocol.AcmePrecision), int(lta.Balance.Uint64()))
	require.Greater(t, int(recovered.Load()), 0,
		"expected the quorum-stuck anchor to be recovered with a collection proof")
}

// TestRangeRecoveryOldRange reproduces the live-network failure where a
// SequenceRange over synthetic messages whose block was anchored long ago
// fails with "receipts cannot be combined" (the collection-proof continuation
// receipt does not chain to the directory root), forcing the slow per-message
// fallback. Unlike TestRangeRecovery, which recovers immediately, this steps
// the network far past the messages' anchor point before requesting the range.
func TestRangeRecoveryOldRange(t *testing.T) {
	var timestamp uint64
	const transfers = 6

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	globals.Globals = new(NetworkGlobals)
	globals.Globals.MajorBlockSchedule = "* * * * *" // a major block every minute (60 minor blocks)
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, globals),
	)

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	alicePart, err := sim.Router().RouteAccount(aliceUrl)
	require.NoError(t, err)
	var bobUrl *url.URL
	var bobPart string
	for i := 0; ; i++ {
		bob := acctesting.GenerateKey("Bob", i)
		bobUrl = acctesting.AcmeLiteAddressStdPriv(bob)
		bobPart, err = sim.Router().RouteAccount(bobUrl)
		require.NoError(t, err)
		if alicePart != bobPart {
			break
		}
	}
	MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

	// Produce a run of cross-partition synthetic deposits and let them deliver.
	st := make([]*protocol.TransactionStatus, transfers)
	for i := range st {
		st[i] = sim.SubmitTxnSuccessfully(MustBuild(t,
			build.Transaction().For(aliceUrl).
				SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
				SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
	}
	for _, st := range st {
		sim.StepUntilN(200, Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}

	// Age the messages: run the network far past their anchor point so the
	// directory has recorded many more anchors for alice's partition. Poke it
	// with a self-transfer every so often so anchors keep being produced.
	sim.StepUntilN(200, MajorBlock(1))
	for i := 0; i < 40; i++ {
		sim.SubmitTxnSuccessfully(MustBuild(t,
			build.Transaction().For(aliceUrl).
				SendTokens(1, protocol.AcmePrecisionPower).To(aliceUrl).
				SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
		sim.StepN(10)
	}
	sim.StepUntilN(200, MajorBlock(2))

	// Now recover the OLD range directly. This is the exact call the executor's
	// healing path makes; on the live network it errors with "receipts cannot
	// be combined" and falls back to per-message.
	ranger, ok := sim.S.Services().Private().(private.SequenceRanger)
	require.True(t, ok, "private client does not serve sequence ranges")
	records, err := ranger.SequenceRange(context.Background(),
		protocol.PartitionUrl(alicePart).JoinPath(protocol.Synthetic),
		protocol.PartitionUrl(bobPart), 1, transfers, private.SequenceOptions{})
	require.NoError(t, err, "range recovery of an old anchored run must not fail")
	require.NotEmpty(t, records)

	// The whole run must be covered by one shared collection proof.
	list := records[len(records)-1].SourceReceiptList
	require.NotNil(t, list, "the range must carry a collection proof")
	require.True(t, list.Validate(nil), "the collection proof must be valid")
}

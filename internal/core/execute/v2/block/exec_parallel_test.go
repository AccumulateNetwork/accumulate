// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4145 shard assignment and envelope classification. The equivalence gate
// lives in test/e2e/sharded_execution_test.go; these pin the pieces it rests
// on.

func userTxn(principal *url.URL) *messaging.TransactionMessage {
	txn := new(protocol.Transaction)
	txn.Header.Principal = principal
	txn.Body = &protocol.SendTokens{}
	return &messaging.TransactionMessage{Transaction: txn}
}

func userSig(signer *url.URL, txid *url.TxID) *messaging.SignatureMessage {
	return &messaging.SignatureMessage{
		Signature: &protocol.ED25519Signature{Signer: signer, PublicKey: make([]byte, 32)},
		TxID:      txid,
	}
}

// classify runs envelopeIdentity against an empty store — resolution can
// only find transactions travelling in the envelope itself.
func classify(t *testing.T, messages ...messaging.Message) (*url.URL, bool) {
	t.Helper()
	return classifyWith(t, nil, messages...)
}

// classifyWith seeds the message store first — the multisig shape, where the
// transaction landed in an earlier block and only the signature travels now.
func classifyWith(t *testing.T, stored []messaging.Message, messages ...messaging.Message) (*url.URL, bool) {
	t.Helper()
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)
	for _, msg := range stored {
		require.NoError(t, batch.Message(msg.Hash()).Main().Put(msg))
	}
	b := new(Block)
	return b.envelopeIdentity(batch, messages)
}

func TestShardAssignment_SameIdentityDifferentPathsSameShard(t *testing.T) {
	// alice.acme/tokens and alice.acme/book never split, at any shard count.
	a := protocol.AccountUrl("alice", "tokens")
	b := protocol.AccountUrl("alice", "book")
	for _, n := range []uint64{2, 4, 8, 64} {
		assert.Equal(t, a.Routing()%n, b.Routing()%n, "shard count %d", n)
	}

	id1, ok1 := classify(t, userTxn(a))
	id2, ok2 := classify(t, userTxn(b))
	require.True(t, ok1 && ok2)
	assert.True(t, id1.Equal(id2), "both accounts classify to the same identity")
}

func TestShardAssignment_LiteTokenAccountAndLiteIdentitySameShard(t *testing.T) {
	lid := protocol.LiteAuthorityForKey(make([]byte, 32), protocol.SignatureTypeED25519)
	lta := lid.JoinPath("ACME")
	for _, n := range []uint64{2, 4, 8, 64} {
		assert.Equal(t, lid.Routing()%n, lta.Routing()%n)
	}
}

func TestShardAssignment_DistinctIdentitiesSpreadAcrossShards(t *testing.T) {
	const n = 8
	seen := map[uint64]bool{}
	for i := 0; i < 64; i++ {
		id := protocol.AccountUrl("identity-" + string(rune('a'+i%26)) + string(rune('a'+i/26)))
		seen[id.Routing()%n] = true
	}
	assert.Greater(t, len(seen), n/2,
		"64 identities must land on more than half of %d shards — assignment is a hash, not a constant", n)
}

// The same function that assigns an account to its partition assigns it to
// its shard: u.Routing(), taken mod the shard count.
func TestShardAssignment_AssignmentUsesRoutingNumber(t *testing.T) {
	id, ok := classify(t, userTxn(protocol.AccountUrl("alice", "tokens")))
	require.True(t, ok)
	assert.Equal(t, protocol.AccountUrl("alice").Routing(), id.Routing(),
		"the classified identity routes exactly as the account's root identity does")
}

func TestClassify_SingleIdentityBundleIsSharded(t *testing.T) {
	txn := userTxn(protocol.AccountUrl("alice", "tokens"))
	sig := userSig(protocol.AccountUrl("alice", "book", "1"), txn.Transaction.ID())

	id, ok := classify(t, sig, txn)
	require.True(t, ok, "the common self-signed bundle must stay parallel")
	assert.True(t, id.Equal(protocol.AccountUrl("alice")))
}

func TestClassify_MultiIdentityBundleGoesSerial(t *testing.T) {
	_, ok := classify(t,
		userTxn(protocol.AccountUrl("alice", "tokens")),
		userTxn(protocol.AccountUrl("bob", "tokens")))
	assert.False(t, ok, "an envelope spanning identities executes serially (hazard ii)")
}

func TestClassify_CrossIdentitySignatureGoesSerial(t *testing.T) {
	txn := userTxn(protocol.AccountUrl("alice", "tokens"))
	sig := userSig(protocol.AccountUrl("bob", "book", "1"), txn.Transaction.ID())

	_, ok := classify(t, sig)
	assert.False(t, ok,
		"a signature for another identity's transaction is a multi-identity bundle — delegation and cross-ADI multisig go serial")
}

func TestClassify_SystemAndAnchorAndSyntheticGoSerial(t *testing.T) {
	// A block anchor.
	anchor := &messaging.BlockAnchor{
		Anchor:    &messaging.SequencedMessage{Message: userTxn(protocol.AccountUrl("x"))},
		Signature: &protocol.ED25519Signature{PublicKey: make([]byte, 32)},
	}
	_, ok := classify(t, anchor)
	assert.False(t, ok, "anchors execute serially (hazard i)")

	// A synthetic message.
	synth := &messaging.SyntheticMessage{Message: &messaging.SequencedMessage{Message: userTxn(protocol.AccountUrl("x"))}}
	_, ok = classify(t, synth)
	assert.False(t, ok, "synthetic deliveries execute serially — which also keeps the MessageIsReady cascade serial (hazard iii)")

	// A system transaction.
	sys := new(protocol.Transaction)
	sys.Header.Principal = protocol.PartitionUrl("BVN0").JoinPath(protocol.Ledger)
	sys.Body = &protocol.DirectoryAnchor{}
	_, ok = classify(t, &messaging.TransactionMessage{Transaction: sys})
	assert.False(t, ok, "system transactions execute serially (hazard i)")

	// A user transaction against a partition identity.
	part := userTxn(protocol.PartitionUrl("BVN0").JoinPath("foo"))
	_, ok = classify(t, part)
	assert.False(t, ok, "partition identities are the serial lane even for user transaction types")

	// A user transaction against ACME.
	acme := userTxn(protocol.AcmeUrl())
	_, ok = classify(t, acme)
	assert.False(t, ok, "ACME is written by system production — serial")
}

// #4149: classification never trusts a submitter's claims.

func remoteStub(claimedPrincipal *url.URL, hash [32]byte) *messaging.TransactionMessage {
	txn := new(protocol.Transaction)
	txn.Header.Principal = claimedPrincipal
	txn.Body = &protocol.RemoteTransaction{Hash: hash}
	return &messaging.TransactionMessage{Transaction: txn}
}

// The attack from #4149: bob's transaction is pending; a crafted envelope
// claims alice everywhere the classifier used to look. Classification must
// resolve the REAL transaction and go serial (signer alice + principal bob =
// multi-identity), never shard to alice.
func TestClassify_LyingSignatureTxIDResolvesTheRealPrincipal(t *testing.T) {
	bobTxn := userTxn(protocol.AccountUrl("bob", "tokens"))
	h := bobTxn.Transaction.ID().Hash()

	lyingSig := userSig(protocol.AccountUrl("alice", "book", "1"),
		protocol.AccountUrl("alice", "x").WithTxID(h))
	lyingStub := remoteStub(protocol.AccountUrl("alice", "x"), h)

	_, ok := classifyWith(t, []messaging.Message{bobTxn}, lyingSig, lyingStub)
	assert.False(t, ok,
		"a signature claiming alice for bob's transaction must not shard to alice")
}

// A remote stub's claimed principal is ignored outright: the envelope
// classifies by the REAL transaction's identity, so its writes happen on the
// right shard no matter what the stub says.
func TestClassify_RemoteStubClaimedPrincipalIsIgnored(t *testing.T) {
	bobTxn := userTxn(protocol.AccountUrl("bob", "tokens"))
	h := bobTxn.Transaction.ID().Hash()

	id, ok := classifyWith(t, []messaging.Message{bobTxn},
		remoteStub(protocol.AccountUrl("alice", "x"), h))
	require.True(t, ok)
	assert.True(t, id.Equal(protocol.AccountUrl("bob")),
		"the real transaction's identity wins, not the stub's claim")
}

// A signature whose transaction is nowhere to be found cannot be classified
// — the executor would load it by hash, so an unresolvable hash is serial.
func TestClassify_UnresolvableSignatureGoesSerial(t *testing.T) {
	sig := userSig(protocol.AccountUrl("alice", "book", "1"),
		protocol.AccountUrl("alice", "x").WithTxID([32]byte{1, 2, 3}))
	_, ok := classify(t, sig)
	assert.False(t, ok, "an unresolvable transaction reference is a serial barrier")

	_, ok = classify(t, remoteStub(protocol.AccountUrl("alice", "x"), [32]byte{4, 5, 6}))
	assert.False(t, ok, "an unresolvable remote stub is a serial barrier")
}

// The multisig shape: the transaction landed in an earlier block, only the
// signature travels now. Resolution finds it in the store and the envelope
// shards by the REAL principal's identity.
func TestClassify_LaterSignatureResolvesFromTheStore(t *testing.T) {
	txn := userTxn(protocol.AccountUrl("alice", "tokens"))
	sig := userSig(protocol.AccountUrl("alice", "book", "1"), txn.Transaction.ID())

	id, ok := classifyWith(t, []messaging.Message{txn}, sig)
	require.True(t, ok)
	assert.True(t, id.Equal(protocol.AccountUrl("alice")))
}

// A held transaction's signatures write the partition ledger's event
// records — a system account — so HoldUntil is a serial barrier (#4149).
func TestClassify_HoldUntilGoesSerial(t *testing.T) {
	txn := userTxn(protocol.AccountUrl("alice", "tokens"))
	txn.Transaction.Header.HoldUntil = &protocol.HoldUntilOptions{MinorBlock: 10}
	sig := userSig(protocol.AccountUrl("alice", "book", "1"), txn.Transaction.ID())

	_, ok := classify(t, txn)
	assert.False(t, ok, "a held transaction is serial")

	_, ok = classifyWith(t, []messaging.Message{txn}, sig)
	assert.False(t, ok, "a signature for a held transaction is serial")
}

// The partition/ACME guard applies to signers too — an operator page
// signature writes partition accounts.
func TestClassify_PartitionSignerGoesSerial(t *testing.T) {
	txn := userTxn(protocol.AccountUrl("alice", "tokens"))
	sig := userSig(protocol.DnUrl().JoinPath(protocol.Operators, "1"), txn.Transaction.ID())

	_, ok := classifyWith(t, []messaging.Message{txn}, sig)
	assert.False(t, ok, "a partition-account signer is the serial lane")
}

// Shard count one takes the plain serial path — no goroutines, no child
// batches, Process called envelope by envelope.
func TestShardCountOfOneIsTheSerialPath(t *testing.T) {
	x := new(Executor)
	x.ExecutionShards = 1
	b := &Block{positions: new(positionCache), Executor: x}

	// With no batch and no state, Process would panic on any real message —
	// an empty envelope exercises exactly the dispatch path and proves it
	// never reached the parallel machinery (which would panic differently).
	results := b.ProcessAll(nil)
	assert.Empty(t, results)
}

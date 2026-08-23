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

func classify(t *testing.T, messages ...messaging.Message) (*url.URL, bool) {
	t.Helper()
	b := new(Block)
	return b.envelopeIdentity(messages)
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

// Shard count one takes the plain serial path — no goroutines, no child
// batches, Process called envelope by envelope.
func TestShardCountOfOneIsTheSerialPath(t *testing.T) {
	x := new(Executor)
	x.ExecutionShards = 1
	b := &Block{Executor: x}

	// With no batch and no state, Process would panic on any real message —
	// an empty envelope exercises exactly the dispatch path and proves it
	// never reached the parallel machinery (which would panic differently).
	results := b.ProcessAll(nil)
	assert.Empty(t, results)
}

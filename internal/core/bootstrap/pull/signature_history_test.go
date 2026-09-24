// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// signedAnchorIn is the transaction and the sequenced message behind the
// signature chain entry of spineWithMessages.
func signedAnchorIn(t *testing.T, b *database.Batch, entry [32]byte) ([32]byte, *messaging.SequencedMessage) {
	t.Helper()
	var ba *messaging.BlockAnchor
	require.NoError(t, b.Message(entry).Main().GetAs(&ba))
	seq := ba.Anchor.(*messaging.SequencedMessage)
	return seq.Message.ID().Hash(), seq
}

// TestFullSpine_WritesEachAnchorSignatureAsExecutionDid — the query API reads
// an anchor's signatures from the pool's history index into its signature
// chain, and the sequence they cover from the transaction's cause
// (internal/api/v3/load.go, loadMessage). Execution writes both beside the
// signature chain entry and no chain holds either, so a pull that took the
// chain alone left a joined node serving every anchor it pulled with no
// signatures (#4413), and one that took the history without the cause served
// signatures over a sequence it could not name (#4416). After the pull the
// node holds what the peer that executed the anchor holds, and a second pull
// of the same account changes nothing.
func TestFullSpine_WritesEachAnchorSignatureAsExecutionDid(t *testing.T) {
	src, u, entries := spineSignedBy(t, signWith(anchorKey), true)
	sigEntry := entries[len(entries)-1]

	dst := newObservedDB(t)
	for pass := 0; pass < 2; pass++ {
		b := dst.Begin(true)
		require.NoError(t, Account(context.Background(), &dbSource{db: src}, b, u, Options{Mode: ModeFullSpine}))
		require.NoError(t, b.Commit())
	}

	s := src.Begin(false)
	defer s.Discard()
	d := dst.Begin(false)
	defer d.Discard()

	txh, seq := signedAnchorIn(t, s, sigEntry)
	wantHist, err := s.Account(u).Transaction(txh).History().Get()
	require.NoError(t, err)
	require.Equal(t, []uint64{0}, wantHist, "precondition: the peer indexes the signature at its chain position")
	gotHist, err := d.Account(u).Transaction(txh).History().Get()
	require.NoError(t, err)
	require.Equal(t, wantHist, gotHist, "the node indexes the anchor's signatures where the peer does")

	wantSigners, err := s.Message(txh).Signers().Get()
	require.NoError(t, err)
	gotSigners, err := d.Message(txh).Signers().Get()
	require.NoError(t, err)
	require.Equal(t, len(wantSigners), len(gotSigners))
	for i := range wantSigners {
		require.True(t, wantSigners[i].Equal(gotSigners[i]), "signer %d", i)
	}

	wantCause, err := s.Message(txh).Cause().Get()
	require.NoError(t, err)
	gotCause, err := d.Message(txh).Cause().Get()
	require.NoError(t, err)
	require.Len(t, gotCause, 1, "the node names the sequence the signatures cover as the anchor's cause")
	require.True(t, wantCause[0].Equal(gotCause[0]))

	// Under the hash it executed with -- the full form's -- and in the form
	// the peer stored it, the transaction by reference.
	full := *seq
	full.Message = &messaging.TransactionMessage{Transaction: mustTransaction(t, s, txh)}
	require.NotEqual(t, full.Hash(), seq.Hash(), "precondition: the stored form does not hash to its key")
	want, err := s.Message(full.Hash()).Main().Get()
	require.NoError(t, err, "precondition: the peer holds the sequence under its hash")
	got, err := d.Message(full.Hash()).Main().Get()
	require.NoError(t, err, "the node holds the sequence under the hash it executed with, not the stored form's")
	require.True(t, messaging.EqualMessage(want, got))

	wantSet, err := s.Account(u).Transaction(txh).ValidatorSignatures().Get()
	require.NoError(t, err)
	gotSet, err := d.Account(u).Transaction(txh).ValidatorSignatures().Get()
	require.NoError(t, err)
	require.Len(t, gotSet, len(wantSet), "the node counts the anchor's quorum from another set than the peer")
	for i := range wantSet {
		require.True(t, protocol.EqualKeySignature(wantSet[i], gotSet[i]), "signature %d", i)
	}

	wantHash, err := s.Account(u).Hash()
	require.NoError(t, err)
	gotHash, err := d.Account(u).Hash()
	require.NoError(t, err)
	require.Equal(t, wantHash, gotHash, "the signature chain is the peer's, not appended to")
}

func mustTransaction(t *testing.T, b *database.Batch, h [32]byte) *protocol.Transaction {
	t.Helper()
	var tm *messaging.TransactionMessage
	require.NoError(t, b.Message(h).Main().GetAs(&tm))
	return tm.Transaction
}

// TestFullSpine_RefusesAnAnchorSignatureThatDoesNotVerify — a signature chain
// entry is believed if it hashes to its entry, and a peer can make an entry
// hash to anything by serving the chain it built (the account is only settled
// against the anchored root afterwards). An anchor whose signature is not a
// signature of the anchor it carries has not served the chain: nothing is
// kept from that peer, and the next peer is asked.
func TestFullSpine_RefusesAnAnchorSignatureThatDoesNotVerify(t *testing.T) {
	cases := map[string]signFunc{
		"a signature of another anchor": func(t *testing.T, seq *messaging.SequencedMessage) protocol.KeySignature {
			other := *seq
			other.Number++
			sig := signWith(anchorKey)(t, &other).(*protocol.ED25519Signature)
			sig.TransactionHash = seq.Hash()
			return sig
		},
		"signature bytes that are no signature": func(t *testing.T, seq *messaging.SequencedMessage) protocol.KeySignature {
			return &protocol.ED25519Signature{
				PublicKey:       anchorKey.Public().(ed25519.PublicKey),
				Signature:       make([]byte, 64),
				Signer:          protocol.DnUrl().JoinPath(protocol.Network),
				TransactionHash: seq.Hash(),
			}
		},
	}
	for name, sign := range cases {
		t.Run(name, func(t *testing.T) {
			liar, u, entries := spineSignedBy(t, sign, false)
			honest, _, _ := spineWithMessages(t)

			dst := newObservedDB(t)
			b := dst.Begin(true)
			defer b.Discard()
			_, _, err := FetchFrom(context.Background(), []Source{&dbSource{db: liar}}, b, u, Options{Mode: ModeFullSpine})
			require.Error(t, err, "a peer that served %s was believed", name)
			require.Contains(t, err.Error(), "does not verify")
			_, err = b.Message(entries[len(entries)-1]).Main().Get()
			require.Error(t, err, "a refused peer's anchor was kept")

			p, i, err := FetchFrom(context.Background(), []Source{&dbSource{db: liar}, &dbSource{db: honest}}, b, u, Options{Mode: ModeFullSpine})
			require.NoError(t, err)
			require.Equal(t, 1, i, "the next peer answered")
			require.NoError(t, p.Keep())
		})
	}
}

// TestCheckAnchorSignature_TakesTheFormsTheExecutorTakes — the Directory signs
// one canonical anchor and every BVN reuses its signatures, so the copy in a
// BVN's pool carries a signature made over the Directory's destination and
// principal (msg_block_anchor.go, checkSignature). A check that took only the
// anchor's own form would refuse the honest history of every BVN pool.
func TestCheckAnchorSignature_TakesTheFormsTheExecutorTakes(t *testing.T) {
	bvnPool := protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
	anchorFor := func(principal, dest *url.URL, body protocol.TransactionBody) *messaging.SequencedMessage {
		txn := new(protocol.Transaction)
		txn.Header.Principal = principal
		txn.Body = body
		return &messaging.SequencedMessage{Message: &messaging.TransactionMessage{Transaction: txn}, Source: protocol.DnUrl(), Destination: dest, Number: 3}
	}
	dirAnchor := &protocol.DirectoryAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: protocol.DnUrl(), MinorBlockIndex: 9}}

	// The copy in BVN0's pool, and the Directory's canonical form it was signed in.
	bvnCopy := anchorFor(bvnPool, protocol.PartitionUrl("BVN0"), dirAnchor)
	dnForm := anchorFor(protocol.DnUrl().JoinPath(protocol.AnchorPool), protocol.DnUrl(), dirAnchor)

	require.NoError(t, checkAnchorSignature(&messaging.BlockAnchor{Anchor: bvnCopy, Signature: signWith(anchorKey)(t, bvnCopy)}),
		"a signature over the anchor's own form")
	require.NoError(t, checkAnchorSignature(&messaging.BlockAnchor{Anchor: bvnCopy, Signature: signWith(anchorKey)(t, dnForm)}),
		"the Directory's signature reused on a BVN's copy")

	// The reuse is the Directory's anchor only: a BVN's anchor signed in
	// another partition's form is not a signature of it.
	bvnAnchor := &protocol.BlockValidatorAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: protocol.PartitionUrl("BVN1"), MinorBlockIndex: 9}}
	own := anchorFor(bvnPool, protocol.PartitionUrl("BVN0"), bvnAnchor)
	other := anchorFor(protocol.DnUrl().JoinPath(protocol.AnchorPool), protocol.DnUrl(), bvnAnchor)
	require.Error(t, checkAnchorSignature(&messaging.BlockAnchor{Anchor: own, Signature: signWith(anchorKey)(t, other)}))

	require.Error(t, checkAnchorSignature(&messaging.BlockAnchor{Anchor: own}), "neither a signature nor a proof")
}

// TestFullSpine_CountsAnAnchorBelowItsQuorumAsThePeerDoes — #4416 review F1.
// The executor counts an anchor's quorum from its validator signature set,
// and an anchor below its quorum is not pending, so the set is under no hash
// the pull verifies. A node that joins while an anchor has one of its copies
// and not the next holds no set for it: when the next copy arrives, on every
// node alike, the peers reach the quorum and execute and the joined node
// counts one and holds (TestAJoinedNodeCountsAStraddlingAnchorAsItsPeersDo).
// The pull rebuilds the set from the history entries, and writes no sequence
// and no cause: below its quorum the sequence has not executed.
func TestFullSpine_CountsAnAnchorBelowItsQuorumAsThePeerDoes(t *testing.T) {
	src, u, entries := spineSignedBy(t, signWith(anchorKey), false)

	dst := newObservedDB(t)
	b := dst.Begin(true)
	require.NoError(t, Account(context.Background(), &dbSource{db: src}, b, u, Options{Mode: ModeFullSpine}))
	require.NoError(t, b.Commit())

	s := src.Begin(false)
	defer s.Discard()
	d := dst.Begin(false)
	defer d.Discard()
	txh, _ := signedAnchorIn(t, s, entries[len(entries)-1])

	want, err := s.Account(u).Transaction(txh).ValidatorSignatures().Get()
	require.NoError(t, err)
	require.Len(t, want, 1, "precondition: the peer holds one signature")
	got, err := d.Account(u).Transaction(txh).ValidatorSignatures().Get()
	require.NoError(t, err)
	require.Len(t, got, 1, "the joined node counts the anchor's signatures from an empty set")
	require.True(t, protocol.EqualKeySignature(want[0], got[0]))

	cause, err := d.Message(txh).Cause().Get()
	require.NoError(t, err)
	require.Empty(t, cause, "a cause written for an anchor that has not executed")
}

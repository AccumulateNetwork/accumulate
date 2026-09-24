// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A node whose key is not active on its partition signs no healing answer:
// no destination accepts that signature, and the requester puts every
// signature of an anchor in one envelope, which the destination refuses whole
// on the first one it cannot accept (#4424; run 20260924T093936Z, 141
// refusals "key is not an active validator", every one the follower's key).
// It still answers an anchor with the quorum signatures it holds, and says
// "not yet" when it holds none, so the requester asks the next node.
func TestSequencer_OutsiderSignsNoAnswer(t *testing.T) {
	const part = "BVN0"
	bvn1 := protocol.PartitionUrl("BVN1")
	pub := func(k ed25519.PrivateKey) []byte { return k.Public().(ed25519.PublicKey) }
	_, member, _ := ed25519.GenerateKey(nil)
	_, other, _ := ed25519.GenerateKey(nil)
	_, outsider, _ := ed25519.GenerateKey(nil)

	// The cache: one synthetic for BVN1 and two anchors, all past the
	// in-flight window.
	cache := synthcache.New(0)
	tx := cache.Begin(7)
	seq := &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: &protocol.Transaction{Header: protocol.TransactionHeader{Principal: protocol.AccountUrl("alice")}, Body: &protocol.SyntheticDepositCredits{Amount: 1}}},
		Source:      protocol.PartitionUrl(part),
		Destination: bvn1,
		Number:      1,
	}
	tx.Add(&synthcache.Entry{Stream: bvn1, Number: 1, Index: 0, Block: 7, Hash: seq.Hash(), Seq: seq})
	anchor := func(n uint64) *protocol.Transaction {
		return &protocol.Transaction{Body: &protocol.BlockValidatorAnchor{PartitionAnchor: protocol.PartitionAnchor{Source: protocol.PartitionUrl(part), MinorBlockIndex: n}}}
	}
	tx.AddAnchor(1, 7, anchor(7))
	tx.AddAnchor(2, 7, anchor(8))
	tx.Commit()
	cache.Begin(7 + synthcache.InFlightBlocks).Commit()

	// Anchor 1's own copy holds another validator's signature; anchor 2's
	// holds none.
	db := database.OpenInMemory(nil)
	own := &protocol.Transaction{Header: protocol.TransactionHeader{Principal: protocol.PartitionUrl(part).JoinPath(protocol.AnchorPool)}, Body: anchor(7).Body}
	held := &protocol.ED25519Signature{PublicKey: pub(other), Signer: protocol.PartitionUrl(part).JoinPath(protocol.Network), TransactionHash: own.ID().Hash()}
	batch := db.Begin(true)
	require.NoError(t, batch.Account(own.Header.Principal).Transaction(own.ID().Hash()).ValidatorSignatures().Put([]protocol.KeySignature{held}))
	require.NoError(t, batch.Commit())

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = protocol.ExecutorVersionLatest
	globals.Network = &protocol.NetworkDefinition{Version: 1}
	globals.Network.AddValidator(pub(member), part, true)
	globals.Network.AddValidator(pub(other), part, true)
	globals.Network.AddValidator(pub(outsider), part, false)

	sequencer := func(key ed25519.PrivateKey) *Sequencer {
		return NewSequencer(SequencerParams{Database: db, EventBus: events.NewBus(nil), Globals: globals, Partition: part, ValidatorKey: key, Cache: cache})
	}
	synth := protocol.PartitionUrl(part).JoinPath(protocol.Synthetic)
	anchors := protocol.PartitionUrl(part).JoinPath(protocol.AnchorPool)
	signers := func(r *api.MessageRecord[messaging.Message]) [][]byte {
		var keys [][]byte
		for _, set := range r.Signatures.Records {
			for _, m := range set.Signatures.Records {
				keys = append(keys, m.Message.(*messaging.SignatureMessage).Signature.(protocol.KeySignature).GetPublicKey())
			}
		}
		return keys
	}
	signedBy := func(r *api.MessageRecord[messaging.Message], k ed25519.PrivateKey) bool {
		for _, s := range signers(r) {
			if bytes.Equal(s, pub(k)) {
				return true
			}
		}
		return false
	}

	// The member signs, as before.
	m := sequencer(member)
	r, err := m.Sequence(context.Background(), synth, bvn1, 1, private.SequenceOptions{})
	require.NoError(t, err)
	require.True(t, signedBy(r, member))
	r, err = m.Sequence(context.Background(), anchors, protocol.DnUrl(), 2, private.SequenceOptions{})
	require.NoError(t, err)
	require.True(t, signedBy(r, member))

	// The outsider does not.
	o := sequencer(outsider)
	_, err = o.Sequence(context.Background(), synth, bvn1, 1, private.SequenceOptions{})
	require.ErrorIs(t, err, errors.NotReady, "an outsider has no signature to give a synthetic")
	_, err = o.SequenceRange(context.Background(), synth, bvn1, 1, 1, private.SequenceOptions{})
	require.ErrorIs(t, err, errors.NotReady)

	r, err = o.Sequence(context.Background(), anchors, protocol.DnUrl(), 1, private.SequenceOptions{})
	require.NoError(t, err, "the outsider holds the quorum's signature for anchor 1")
	require.False(t, signedBy(r, outsider), "the outsider signed a healing answer")
	require.Equal(t, [][]byte{pub(other)}, signers(r), "only what it holds")

	_, err = o.Sequence(context.Background(), anchors, protocol.DnUrl(), 2, private.SequenceOptions{})
	require.ErrorIs(t, err, errors.NotReady, "holding no signature for anchor 2, the outsider has nothing to give")

	// A range answers the prefix it can sign for, and "not yet" when that
	// is nothing.
	rs, err := o.SequenceRange(context.Background(), anchors, protocol.DnUrl(), 1, 2, private.SequenceOptions{})
	require.NoError(t, err)
	require.Len(t, rs, 1)
	require.False(t, signedBy(rs[0], outsider))
	_, err = o.SequenceRange(context.Background(), anchors, protocol.DnUrl(), 2, 2, private.SequenceOptions{})
	require.ErrorIs(t, err, errors.NotReady)
}

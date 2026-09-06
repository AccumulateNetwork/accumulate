// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"crypto/ed25519"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	dbmerkle "gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A collected entry sizes its stream's stage: whatever is held at a number
// appends slots up to that number. The number of a collected entry is not yet
// proven — its proof waits for an anchor that may never come — so it is held
// only on the word of a current validator of its source (#4243, review
// 2026-09-06 finding 26). A self-consistent receipt list over a forged number,
// with a signature nobody in the source's validator set made, is refused and
// holds nothing. The same list under a known anchor still executes on the
// proof alone: the proof authenticates the body, and requiring a current
// validator there wedged historical recovery after churn (#4056).
func TestSyntheticSignerGatesTheHold(t *testing.T) {
	var x SyntheticMessage
	seed := sha256.Sum256([]byte("validator"))
	key := ed25519.NewKeyFromSeed(seed[:])
	strangerSeed := sha256.Sum256([]byte("stranger"))
	stranger := ed25519.NewKeyFromSeed(strangerSeed[:])
	source, dest := protocol.PartitionUrl("foo"), protocol.PartitionUrl("bar")

	// The attacker's message: a real entry re-wrapped with a number a million
	// past anything delivered, and the attacker's own chain containing its
	// hash, so the receipt list is self-consistent.
	const forged = 1_000_000
	seq := &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: &protocol.Transaction{Header: protocol.TransactionHeader{Principal: protocol.AccountUrl("alice")}, Body: &protocol.SyntheticDepositCredits{Amount: 1}}},
		Source:      source,
		Destination: dest,
		Number:      forged,
	}
	hash := seq.Hash()
	store := memory.New(nil)
	tx := store.Begin(nil, true)
	t.Cleanup(tx.Discard)
	chain := dbmerkle.NewChain(nil, keyvalue.RecordStore{Store: tx}, record.NewKey("attacker"), 8, dbmerkle.ChainTypeTransaction, "attacker")
	pad := sha256.Sum256([]byte("pad"))
	require.NoError(t, chain.AddEntry(pad[:], false))
	require.NoError(t, chain.AddEntry(hash[:], false))
	list, err := dbmerkle.GetReceiptList(chain, 0, 1)
	require.NoError(t, err)
	require.True(t, list.Validate(nil))
	proof := &protocol.AnnotatedReceipt{Anchor: &protocol.AnchorMetadata{Account: protocol.DnUrl(), SourceBlock: 5}, ReceiptList: list}

	newMsg := func(signer ed25519.PrivateKey) *messaging.SyntheticMessage {
		sig := &protocol.ED25519Signature{
			PublicKey:       signer[32:],
			Signer:          source.JoinPath(protocol.Network),
			SignerVersion:   1,
			TransactionHash: hash,
		}
		protocol.SignED25519(sig, signer, nil, hash[:])
		return &messaging.SyntheticMessage{Message: seq, Proof: proof, Signature: sig}
	}

	newBlock := func() *Block {
		globals := new(Globals)
		globals.Active.ExecutorVersion = protocol.ExecutorVersionLatest
		globals.Active.Network = &protocol.NetworkDefinition{
			Partitions: []*protocol.PartitionInfo{{ID: "bar", Type: protocol.PartitionTypeBlockValidator}},
			Validators: []*protocol.ValidatorInfo{{
				PublicKey:     key[32:],
				PublicKeyHash: sha256.Sum256(key[32:]),
				Partitions:    []*protocol.ValidatorPartitionInfo{{ID: "foo", Active: true}},
			}},
		}
		b := &Block{
			positions: new(positionCache),
			staging:   execute.NewStaging().Begin(),
			Executor: &Executor{
				messageExecutors: map[messaging.MessageType]ExecutorFactory2[messaging.MessageType, *MessageContext]{
					messaging.MessageTypeSequenced: func(*MessageContext) (ExecutorFor[messaging.MessageType, *MessageContext], bool) {
						return fakeExecutor{}, true
					},
				},
			},
		}
		b.Executor.Describe.PartitionId = "bar"
		b.Executor.globalsPtr.Store(globals)
		return b
	}
	process := func(t *testing.T, b *Block, msg messaging.Message, anchored bool) *protocol.TransactionStatus {
		t.Helper()
		ctx := &MessageContext{message: msg, bundle: &bundle{Block: b}}
		db := database.OpenInMemory(nil)
		batch := db.Begin(true)
		t.Cleanup(batch.Discard)
		if anchored {
			require.NoError(t, batch.Account(b.Executor.Describe.AnchorPool()).AnchorChain(protocol.Directory).Root().Inner().AddEntry(proof.TerminalAnchor(), false))
		}
		st, err := x.Process(batch, ctx)
		require.NoError(t, err)
		return st
	}
	stream := execute.StreamID{Ledger: dest.JoinPath(protocol.Synthetic), Source: source}

	t.Run("a stranger's forged number holds nothing", func(t *testing.T) {
		b := newBlock()
		st := process(t, b, newMsg(stranger), false)
		require.Error(t, st.AsError(), "refused, not pending")
		require.ErrorIs(t, st.AsError(), errors.BadRequest)
		require.Zero(t, b.staging.Sighted(stream), "the stage did not grow")
		_, held := b.staging.IDOf(stream, forged)
		require.False(t, held)
	})

	t.Run("a garbage signature holds nothing", func(t *testing.T) {
		b := newBlock()
		msg := newMsg(key)
		msg.Signature.(*protocol.ED25519Signature).Signature = []byte("garbage")
		st := process(t, b, msg, false)
		require.ErrorIs(t, st.AsError(), errors.BadRequest)
		require.Zero(t, b.staging.Sighted(stream))
	})

	t.Run("a validator's word is collected", func(t *testing.T) {
		b := newBlock()
		st := process(t, b, newMsg(key), false)
		require.NoError(t, st.AsError())
		require.Equal(t, errors.Pending, st.Code)
		_, held := b.staging.IDOf(stream, forged)
		require.True(t, held, "held at its number, collected")
	})

	t.Run("an anchored proof executes whoever signed", func(t *testing.T) {
		b := newBlock()
		msg := newMsg(stranger)
		st := process(t, b, msg, true)
		require.NoError(t, st.AsError())
		require.Equal(t, errors.Delivered, st.Code, "the proof authenticates the body (#4056)")
	})
}

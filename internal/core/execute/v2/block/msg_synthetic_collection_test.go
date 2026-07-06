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
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	dbmerkle "gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestSyntheticCollectionProof exercises the dual-mode proof branch (#4048): a
// synthetic message carrying a collection proof (ReceiptList) instead of an
// individual receipt.
func TestSyntheticCollectionProof(t *testing.T) {
	var x SyntheticMessage

	// Set up the message
	seed := sha256.Sum256([]byte("validator"))
	key := ed25519.NewKeyFromSeed(seed[:])
	seq := &messaging.SequencedMessage{
		Message:     new(messaging.TransactionMessage),
		Source:      protocol.PartitionUrl("foo"),
		Destination: protocol.PartitionUrl("bar"),
		Number:      10,
	}
	hash := seq.Hash()

	// Build a real collection proof over a chain that contains the message
	// hash among other entries
	store := memory.New(nil)
	tx := store.Begin(nil, true)
	t.Cleanup(tx.Discard)
	src := dbmerkle.NewChain(nil, keyvalue.RecordStore{Store: tx}, record.NewKey("src"), 8, dbmerkle.ChainTypeTransaction, "src")
	other1, other2 := sha256.Sum256([]byte("one")), sha256.Sum256([]byte("two"))
	for _, h := range [][]byte{other1[:], hash[:], other2[:]} {
		require.NoError(t, src.AddEntry(h, false))
	}
	list, err := dbmerkle.GetReceiptList(src, 0, 2)
	require.NoError(t, err)
	require.True(t, list.Validate(nil))

	newMsg := func(proof *protocol.AnnotatedReceipt) *messaging.SyntheticMessage {
		syn := &messaging.SyntheticMessage{Message: seq, Proof: proof}
		sig := &protocol.ED25519Signature{
			PublicKey:       key[32:],
			Signer:          protocol.DnUrl().JoinPath(protocol.Network),
			SignerVersion:   1,
			TransactionHash: seq.Hash(),
		}
		protocol.SignED25519(sig, key, nil, hash[:])
		syn.Signature = sig
		return syn
	}
	collectionProof := &protocol.AnnotatedReceipt{
		Anchor:      &protocol.AnchorMetadata{Account: protocol.UnknownUrl()},
		ReceiptList: list,
	}

	// Set up the context
	newBlock := func(version protocol.ExecutorVersion) *Block {
		globals := new(Globals)
		globals.Active.ExecutorVersion = version
		globals.Active.Network = &protocol.NetworkDefinition{
			Partitions: []*protocol.PartitionInfo{{
				ID:   "bar",
				Type: protocol.PartitionTypeBlockValidator,
			}},
			Validators: []*protocol.ValidatorInfo{{
				PublicKey:     key[32:],
				PublicKeyHash: sha256.Sum256(key[32:]),
				Partitions: []*protocol.ValidatorPartitionInfo{{
					ID:     "foo",
					Active: true,
				}},
			}},
		}
		return &Block{
			Executor: &Executor{
				globals: globals,
				messageExecutors: map[messaging.MessageType]ExecutorFactory2[messaging.MessageType, *MessageContext]{
					messaging.MessageTypeSequenced: func(*MessageContext) (ExecutorFor[messaging.MessageType, *MessageContext], bool) {
						return fakeExecutor{}, true
					},
				},
			},
		}
	}

	run := func(t *testing.T, block *Block, msg messaging.Message, anchor bool, method func(SyntheticMessage, *database.Batch, *MessageContext) (*protocol.TransactionStatus, error)) (*protocol.TransactionStatus, error) {
		t.Helper()
		ctx := &MessageContext{message: msg, bundle: &bundle{Block: block}}
		db := database.OpenInMemory(nil)
		batch := db.Begin(true)
		t.Cleanup(batch.Discard)
		if anchor {
			require.NoError(t, batch.Account(ctx.Executor.Describe.AnchorPool()).
				AnchorChain(protocol.Directory).
				Root().
				Inner().
				AddEntry(collectionProof.TerminalAnchor(), false))
		}
		return method(x, batch, ctx)
	}

	t.Run("rejected before activation", func(t *testing.T) {
		block := newBlock(protocol.ExecutorVersionV2Tanegashima)
		_, err := run(t, block, newMsg(collectionProof), false, SyntheticMessage.Validate)
		require.ErrorContains(t, err, "collection proofs are not enabled")
	})

	t.Run("rejected with both proof forms", func(t *testing.T) {
		block := newBlock(protocol.ExecutorVersionVNext)
		both := collectionProof.Copy()
		both.Receipt = &dbmerkle.Receipt{Start: hash[:], Anchor: hash[:]}
		_, err := run(t, block, newMsg(both), false, SyntheticMessage.Validate)
		require.ErrorContains(t, err, "not both")
	})

	t.Run("rejected if the message is not included", func(t *testing.T) {
		block := newBlock(protocol.ExecutorVersionVNext)
		shortList, err := dbmerkle.GetReceiptList(src, 0, 0) // excludes the message hash
		require.NoError(t, err)
		proof := &protocol.AnnotatedReceipt{
			Anchor:      &protocol.AnchorMetadata{Account: protocol.UnknownUrl()},
			ReceiptList: shortList,
		}
		_, err = run(t, block, newMsg(proof), false, SyntheticMessage.Validate)
		require.ErrorContains(t, err, "does not include")
	})

	t.Run("validates once enabled", func(t *testing.T) {
		block := newBlock(protocol.ExecutorVersionVNext)
		_, err := run(t, block, newMsg(collectionProof), false, SyntheticMessage.Validate)
		require.NoError(t, err)
	})

	t.Run("processes with a known anchor", func(t *testing.T) {
		block := newBlock(protocol.ExecutorVersionVNext)
		status, err := run(t, block, newMsg(collectionProof), true, SyntheticMessage.Process)
		require.NoError(t, err)
		require.NoError(t, status.AsError())
	})

	t.Run("process rejects an unknown anchor", func(t *testing.T) {
		block := newBlock(protocol.ExecutorVersionVNext)
		status, err := run(t, block, newMsg(collectionProof), false, SyntheticMessage.Process)
		require.NoError(t, err)
		require.ErrorContains(t, status.AsError(), "not a known directory anchor")
	})
}

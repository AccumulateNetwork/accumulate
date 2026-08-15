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
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestSyntheticCollectionProof exercises the dual-mode proof branch (#4087): a
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
		block := newBlock(protocol.ExecutorVersionV2Jiuquan)
		_, err := run(t, block, newMsg(collectionProof), false, SyntheticMessage.Validate)
		require.ErrorContains(t, err, "collection proofs are not enabled")
	})

	t.Run("rejected with both proof forms", func(t *testing.T) {
		block := newBlock(protocol.ExecutorVersionV2Kourou)
		both := collectionProof.Copy()
		both.Receipt = &dbmerkle.Receipt{Start: hash[:], Anchor: hash[:]}
		_, err := run(t, block, newMsg(both), false, SyntheticMessage.Validate)
		require.ErrorContains(t, err, "both a receipt and a receipt list")
	})

	t.Run("rejected if the message is not included", func(t *testing.T) {
		block := newBlock(protocol.ExecutorVersionV2Kourou)
		shortList, err := dbmerkle.GetReceiptList(src, 0, 0) // excludes the message hash
		require.NoError(t, err)
		proof := &protocol.AnnotatedReceipt{
			Anchor:      &protocol.AnchorMetadata{Account: protocol.UnknownUrl()},
			ReceiptList: shortList,
		}
		_, err = run(t, block, newMsg(proof), false, SyntheticMessage.Validate)
		require.ErrorContains(t, err, "is not included in the collection proof")
	})

	t.Run("validates once enabled", func(t *testing.T) {
		block := newBlock(protocol.ExecutorVersionV2Kourou)
		_, err := run(t, block, newMsg(collectionProof), false, SyntheticMessage.Validate)
		require.NoError(t, err)
	})

	t.Run("processes with a known anchor", func(t *testing.T) {
		block := newBlock(protocol.ExecutorVersionV2Kourou)
		status, err := run(t, block, newMsg(collectionProof), true, SyntheticMessage.Process)
		require.NoError(t, err)
		require.NoError(t, status.AsError())
	})

	t.Run("process leaves an unknown anchor pending, then retries", func(t *testing.T) {
		// Before activation an unknown anchor is a terminal failure; once
		// collection proofs are active it must stay retryable (#4087)
		block := newBlock(protocol.ExecutorVersionV2Kourou)
		msg := newMsg(collectionProof)
		ctx := &MessageContext{message: msg, bundle: &bundle{Block: block}}
		db := database.OpenInMemory(nil)
		batch := db.Begin(true)
		t.Cleanup(batch.Discard)

		// Without the anchor the message is recorded pending, not failed
		status, err := x.Process(batch, ctx)
		require.NoError(t, err)
		require.Equal(t, errors.Pending, status.Code)

		// Once the anchor arrives, reprocessing the same message delivers it
		require.NoError(t, batch.Account(ctx.Executor.Describe.AnchorPool()).
			AnchorChain(protocol.Directory).
			Root().
			Inner().
			AddEntry(collectionProof.TerminalAnchor(), false))
		ctx = &MessageContext{message: msg, bundle: &bundle{Block: block}}
		status, err = x.Process(batch, ctx)
		require.NoError(t, err)
		require.Equal(t, errors.Delivered, status.Code)
	})

	t.Run("individual proof with an unknown anchor is still held, not failed", func(t *testing.T) {
		// The individual-proof path must be untouched by this change. From
		// Jiuquan an unknown anchor is HELD and retried in place rather than
		// failing terminally (#4070) — that fix predates collection proofs and
		// must survive them, for both proof forms.
		block := newBlock(protocol.ExecutorVersionV2Jiuquan)
		individual := &protocol.AnnotatedReceipt{
			Anchor:  &protocol.AnchorMetadata{Account: protocol.UnknownUrl()},
			Receipt: &dbmerkle.Receipt{Start: hash[:], Anchor: hash[:]},
		}
		status, err := run(t, block, newMsg(individual), false, SyntheticMessage.Process)
		require.NoError(t, err)
		require.NoError(t, status.AsError(),
			"an unknown anchor must be held for retry, not turned into a terminal failure")
	})
}

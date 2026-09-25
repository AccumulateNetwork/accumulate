// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	stderrors "errors"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	v3impl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	apierrors "gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// swapMessage serves, for every message asked for, another message, or
// nothing when other is nil.
type swapMessage struct {
	Source
	other messaging.Message
}

func (s swapMessage) QueryMessage(_ context.Context, id *url.TxID, _ *api.DefaultQuery) (*api.MessageRecord[messaging.Message], error) {
	if s.other == nil {
		return nil, v3NotFound(id)
	}
	return &api.MessageRecord[messaging.Message]{Message: s.other}, nil
}

// TestAQueuedDeliveryIsPulledWithItsOwnMessage: the synthetic ledger's queue
// is verified through the leaf, and each queued message through its hash
// (#4399). The pull keeps the message the queue names, refuses a peer that
// serves another in its place, and does not read a peer's NotFound for a
// queued message as the peer holding no leaf for the ledger -- which would
// make the join drop <partition>/synthetic for good.
func TestAQueuedDeliveryIsPulledWithItsOwnMessage(t *testing.T) {
	const partitionID = "QueuedMessage"
	part := protocol.PartitionUrl(partitionID)
	synth := part.JoinPath(protocol.Synthetic)
	sysLedger := part.JoinPath(protocol.Ledger)

	deposit := func(n int64) messaging.Message {
		return &messaging.TransactionMessage{Transaction: &protocol.Transaction{
			Header: protocol.TransactionHeader{Principal: protocol.AccountUrl("alice", "tokens")},
			Body:   &protocol.SyntheticDepositTokens{Token: protocol.AcmeUrl(), Amount: *big.NewInt(n)},
		}}
	}
	queued := deposit(1)
	id := protocol.AccountUrl("alice", "tokens").WithTxID(queued.Hash())

	src := newObservedDB(t)
	b := src.Begin(true)
	sl := &protocol.SyntheticLedger{Url: synth}
	require.NoError(t, b.Account(synth).Main().Put(sl))
	require.NoError(t, b.Account(synth).LocalDeliveryQueue().Add(id))
	require.NoError(t, b.Message(queued.Hash()).Main().Put(queued))
	ledger := &protocol.SystemLedger{Url: sysLedger, Index: 9}
	require.NoError(t, b.Account(sysLedger).Main().Put(ledger))
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
	b = src.Begin(true)
	idx, err := b.Account(sysLedger).RootChain().Index().Get()
	require.NoError(t, err)
	entry, err := (&protocol.IndexEntry{BlockIndex: ledger.Index}).MarshalBinary()
	require.NoError(t, err)
	require.NoError(t, idx.AddEntry(entry, false))
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())

	honest := api.Querier2{Querier: v3impl.NewQuerier(v3impl.QuerierParams{Database: src, Partition: partitionID})}
	opts := Options{Mode: ModeStateOnly, WithReceipt: true, Partition: part}
	ctx := context.Background()

	t.Run("honest", func(t *testing.T) {
		dst := newObservedDB(t)
		batch := dst.Begin(true)
		defer batch.Discard()
		require.NoError(t, Account(ctx, honest, batch, synth, opts))
		msg, err := batch.Message(id.Hash()).Main().Get()
		require.NoError(t, err, "the queued delivery was pulled without its message")
		require.Equal(t, id.Hash(), msg.Hash())
	})

	t.Run("another message in its place is refused", func(t *testing.T) {
		dst := newObservedDB(t)
		batch := dst.Begin(true)
		defer batch.Discard()
		require.Error(t, Account(ctx, swapMessage{Source: honest, other: deposit(2)}, batch, synth, opts),
			"a message that does not hash to the queued ID was kept")
		_, err := batch.Message(deposit(2).Hash()).Main().Get()
		require.Error(t, err, "the refused message was written")
	})

	t.Run("a missing message is not a missing leaf", func(t *testing.T) {
		dst := newObservedDB(t)
		batch := dst.Begin(true)
		defer batch.Discard()
		_, _, err := FetchFrom(ctx, []Source{swapMessage{Source: honest}}, batch, synth, opts)
		require.Error(t, err)
		require.False(t, stderrors.Is(err, ErrNoLeaf),
			"a NotFound for a queued message was read as the peer holding no leaf for the ledger: %v", err)
	})
}

func v3NotFound(id *url.TxID) error {
	return apierrors.NotFound.WithFormat("message %v not found", id)
}

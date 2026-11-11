// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// processNetworkWrite runs processNetworkAccountUpdates for a WriteData of the
// given network definition to dn.acme/network, on a Directory executor at the
// given version whose globals hold definition version 1.
func processNetworkWrite(t *testing.T, version protocol.ExecutorVersion, def *protocol.NetworkDefinition) (*Executor, *core.GlobalValues, error) {
	t.Helper()
	x := new(Executor)
	x.Describe = execute.DescribeShim{NetworkType: protocol.PartitionTypeDirectory, PartitionId: protocol.Directory}
	g := core.NewGlobals(&core.GlobalValues{ExecutorVersion: version, Network: &protocol.NetworkDefinition{Version: 1}, Routing: new(protocol.RoutingTable)})
	x.globalsPtr.Store(&Globals{Active: *g, Pending: *g.Copy()})

	values := core.GlobalValues{Network: def}
	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.DnUrl().JoinPath(protocol.Network)
	txn.Body = &protocol.WriteData{Entry: values.FormatNetwork(), WriteToState: true}
	principal := &protocol.DataAccount{Url: txn.Header.Principal}

	batch := database.OpenInMemory(nil).Begin(true)
	t.Cleanup(batch.Discard)
	require.NoError(t, batch.Account(x.Describe.Ledger()).Main().Put(&protocol.SystemLedger{Url: x.Describe.Ledger()}))
	st := chain.NewStateManager(x.Describe, &x.globals().Active, nil, batch.Begin(true), principal, txn, nil)
	t.Cleanup(st.Discard)

	pending, err := x.processNetworkAccountUpdates(st, &chain.Delivery{Transaction: txn}, principal)
	return x, pending, err
}

// From V2Canaveral a network write changes nothing in memory while it is
// processed: the new pending globals come back to the caller, which applies
// them only after the transaction's commit and record succeed. So a commit
// that fails after the parse leaves the executor's globals as they were
// (#4615).
func TestNetworkAccountUpdate_StagedUntilSuccess(t *testing.T) {
	x, pending, err := processNetworkWrite(t, protocol.ExecutorVersionV2Canaveral, &protocol.NetworkDefinition{Version: 2})
	require.NoError(t, err)
	require.NotNil(t, pending, "the change comes back to the caller")
	require.Equal(t, uint64(2), pending.Network.Version)
	require.Equal(t, uint64(1), x.globals().Pending.Network.Version, "pending globals must not change before the transaction succeeds")
	require.Equal(t, uint64(1), x.globals().Active.Network.Version)
}

// A definition whose version does not increase is rejected and changes
// nothing, not even the copy the caller would apply.
func TestNetworkAccountUpdate_StaleVersionChangesNothing(t *testing.T) {
	x, pending, err := processNetworkWrite(t, protocol.ExecutorVersionV2Canaveral, &protocol.NetworkDefinition{Version: 1})
	require.ErrorContains(t, err, "version must increase")
	require.Nil(t, pending)
	require.Equal(t, uint64(1), x.globals().Pending.Network.Version)
}

// Before V2Canaveral the executor wrote the pending globals as it parsed,
// storing even a definition it then rejected. Replaying that history needs the
// same.
func TestNetworkAccountUpdate_BeforeCanaveralWritesPending(t *testing.T) {
	x, pending, err := processNetworkWrite(t, protocol.ExecutorVersionV2Kourou, &protocol.NetworkDefinition{Version: 1, NetworkName: "stale"})
	require.ErrorContains(t, err, "version must increase: 1 <= 1")
	require.Nil(t, pending)
	require.Equal(t, "stale", x.globals().Pending.Network.NetworkName)

	x, pending, err = processNetworkWrite(t, protocol.ExecutorVersionV2Kourou, &protocol.NetworkDefinition{Version: 2})
	require.NoError(t, err)
	require.Nil(t, pending)
	require.Equal(t, uint64(2), x.globals().Pending.Network.Version)
}

// conflictingWrite executes a WriteData through the state manager, and while
// it does commits another write to the same account from a sibling batch, so
// that the state manager's commit fails with a conflict.
type conflictingWrite struct{ parent *database.Batch }

func (conflictingWrite) Type() protocol.TransactionType { return protocol.TransactionTypeWriteData }

func (w conflictingWrite) Validate(st *chain.StateManager, tx *chain.Delivery) (protocol.TransactionResult, error) {
	return w.Execute(st, tx)
}

func (w conflictingWrite) Execute(st *chain.StateManager, tx *chain.Delivery) (protocol.TransactionResult, error) {
	body := tx.Transaction.Body.(*protocol.WriteData)
	account := &protocol.DataAccount{Url: tx.Transaction.Header.Principal, Entry: body.Entry}
	if err := st.Update(account); err != nil {
		return nil, err
	}

	sibling := w.parent.Begin(true)
	defer sibling.Discard()
	other := &protocol.DataAccount{Url: tx.Transaction.Header.Principal}
	if err := sibling.Account(other.Url).Main().Put(other); err != nil {
		return nil, err
	}
	return nil, sibling.Commit()
}

// A network write whose entry parses but whose state-manager commit fails is
// recorded as failed and leaves the executor's globals as they were, so block
// close has nothing to activate (#4615).
func TestNetworkAccountUpdate_CommitFailureChangesNothing(t *testing.T) {
	x := new(Executor)
	x.Describe = execute.DescribeShim{NetworkType: protocol.PartitionTypeDirectory, PartitionId: protocol.Directory}
	g := core.NewGlobals(&core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV2Canaveral, Network: &protocol.NetworkDefinition{Version: 1}, Routing: new(protocol.RoutingTable)})
	x.globalsPtr.Store(&Globals{Active: *g, Pending: *g.Copy()})

	batch := database.OpenInMemory(nil).Begin(true)
	t.Cleanup(batch.Discard)
	x.executors = map[protocol.TransactionType]chain.TransactionExecutor{protocol.TransactionTypeWriteData: conflictingWrite{batch}}

	values := core.GlobalValues{Network: &protocol.NetworkDefinition{Version: 2}}
	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.DnUrl().JoinPath(protocol.Network)
	txn.Body = &protocol.WriteData{Entry: values.FormatNetwork(), WriteToState: true}
	require.NoError(t, batch.Account(txn.Header.Principal).Main().Put(&protocol.DataAccount{Url: txn.Header.Principal}))
	require.NoError(t, batch.Account(x.Describe.Ledger()).Main().Put(&protocol.SystemLedger{Url: x.Describe.Ledger()}))

	// The transaction is initiated, and arrives inside a synthetic message so
	// it executes without collecting signatures
	payment := &messaging.CreditPayment{Initiator: true, TxID: txn.ID(), Payer: protocol.DnUrl().JoinPath(protocol.Operators, "1")}
	require.NoError(t, batch.Message(payment.Hash()).Main().Put(payment))
	require.NoError(t, batch.Account(txn.Header.Principal).Transaction(txn.ID().Hash()).Payments().Add(payment.Hash()))

	b := &Block{positions: new(positionCache), Executor: x, Batch: batch, staging: x.staging().Begin()}
	ctx := &TransactionContext{
		MessageContext: &MessageContext{
			bundle: &bundle{Block: b, batch: batch},
			parent: &MessageContext{message: new(messaging.SyntheticMessage)},
		},
		transaction: txn,
	}

	status, _, err := ctx.processTransaction(batch)
	require.NoError(t, err)
	require.ErrorIs(t, status.AsError(), errors.Conflict, "the fixture must fail the commit, not something earlier")
	require.Equal(t, uint64(1), x.globals().Pending.Network.Version, "a failed commit must not change the pending globals")
	require.Equal(t, uint64(1), x.globals().Active.Network.Version)
}

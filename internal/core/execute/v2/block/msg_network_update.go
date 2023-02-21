// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[NetworkUpdate](&messageExecutors, internal.MessageTypeNetworkUpdate)
}

// NetworkUpdate constructs a transaction for the network update and queues it
// for processing.
type NetworkUpdate struct{}

func (NetworkUpdate) Process(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	msg, ok := ctx.message.(*internal.NetworkUpdate)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", internal.MessageTypeNetworkUpdate, ctx.message.Type())
	}

	txn := new(protocol.Transaction)
	txn.Header.Principal = msg.Account
	txn.Header.Initiator = msg.Cause
	txn.Body = msg.Body

	batch = batch.Begin(true)
	defer batch.Discard()

	// Record that the cause produced this update
	err := batch.Transaction(msg.Cause[:]).Produced().Add(txn.ID())
	if err != nil {
		return nil, errors.UnknownError.WithFormat("update cause: %w", err)
	}

	// Store the transaction
	err = batch.Message(txn.ID().Hash()).Main().Put(&messaging.UserTransaction{Transaction: txn})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store transaction: %w", err)
	}

	// Store the transaction status
	signer := ctx.Executor.globals.Active.AsSigner(ctx.Executor.Describe.PartitionId)
	status := new(protocol.TransactionStatus)
	status.TxID = txn.ID()
	status.Initiator = signer.GetUrl()
	status.AddSigner(signer)

	err = batch.Transaction(txn.GetHash()).Status().Put(status)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("store transaction status: %w", err)
	}

	// Execute the transaction
	st, err := ctx.callMessageExecutor(batch, ctx.childWith(&messaging.UserTransaction{Transaction: txn}))
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	err = batch.Commit()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return st, nil
}

// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Block struct {
	BlockMeta
	State BlockState
	Batch *database.Batch
}

func (x *Executor) ExecuteEnvelope(block *Block, delivery *chain.Delivery) (*protocol.TransactionStatus, error) {
	if delivery.Transaction.Body.Type() == protocol.TransactionTypeSystemWriteData {
		return nil, errors.BadRequest.WithFormat("a %v transaction cannot be submitted directly", protocol.TransactionTypeSystemWriteData)
	}

	status, additional, err := x.executeEnvelope(block, delivery, false)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	// If the signature failed, fetch the transaction status
	if status == nil {
		status, err = block.Batch.Transaction(delivery.Transaction.GetHash()).Status().Get()
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// Record when the transaction is received
	if status.Received == 0 {
		status.Received = block.Index
		err = block.Batch.Transaction(delivery.Transaction.GetHash()).PutStatus(status)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	// Process additional transactions. This is intentionally non-recursive.
	for len(additional) > 0 {
		var next []*chain.Delivery
		for _, delivery := range additional {
			_, additional, err := x.executeEnvelope(block, delivery, true)
			if err != nil {
				return nil, errors.UnknownError.Wrap(err)
			}

			next = append(next, additional...)
			if err != nil {
				return nil, err
			}
		}
		additional, next = next, nil
	}

	return status, nil
}

func (x *Executor) executeEnvelope(block *Block, delivery *chain.Delivery, additional bool) (*protocol.TransactionStatus, []*chain.Delivery, error) {
	{
		fn := x.logger.Debug
		kv := []interface{}{
			"block", block.Index,
			"type", delivery.Transaction.Body.Type(),
			"txn-hash", logging.AsHex(delivery.Transaction.GetHash()).Slice(0, 4),
			"principal", delivery.Transaction.Header.Principal,
		}
		switch delivery.Transaction.Body.Type() {
		case protocol.TransactionTypeDirectoryAnchor,
			protocol.TransactionTypeBlockValidatorAnchor:
			fn = x.logger.Info
			kv = append(kv, "module", "anchoring")
		}
		if additional {
			fn("Executing additional", kv...)
		} else {
			fn("Executing transaction", kv...)
		}
	}

	r := x.BlockTimers.Start(BlockTimerTypeExecuteEnvelope)
	defer x.BlockTimers.Stop(r)

	status, err := delivery.LoadTransaction(block.Batch)
	switch {
	case err == nil:
		// Ok

	case !errors.Is(err, errors.Delivered):
		// Unknown error
		return nil, nil, errors.UnknownError.Wrap(err)

	default:
		// Transaction has already been delivered
		status := status.Copy()
		status.Code = errors.Delivered
		return status, nil, nil
	}

	// Verify the transaction type is valid
	txnType := delivery.Transaction.Body.Type()
	if txnType != protocol.TransactionTypeRemote {
		if _, ok := x.executors[txnType]; !ok {
			return nil, nil, errors.InternalError.WithFormat("missing executor for %v", txnType)
		}
	}

	err = delivery.LoadSyntheticMetadata(block.Batch, txnType, status)
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}

	// Process signatures
	shouldProcessTransaction := !txnType.IsUser()
	{
		batch := block.Batch.Begin(true)
		defer batch.Discard()

		var didFail bool
		for _, signature := range delivery.Signatures {
			if !signature.Type().IsSystem() && signature.RoutingLocation().LocalTo(delivery.Transaction.Header.Principal) {
				shouldProcessTransaction = true
			}

			status := new(protocol.TransactionStatus)
			status.Received = block.Index

			// TODO: add an ID method to signatures
			sigHash := *(*[32]byte)(signature.Hash())
			switch signature := signature.(type) {
			case protocol.KeySignature:
				status.TxID = signature.GetSigner().WithTxID(sigHash)
			case *protocol.ReceiptSignature, *protocol.InternalSignature:
				status.TxID = delivery.Transaction.Header.Principal.WithTxID(sigHash)
			default:
				status.TxID = signature.RoutingLocation().WithTxID(sigHash)
			}

			s, err := x.ProcessSignature(batch, delivery, signature)
			if err == nil {
				status.Code = errors.Delivered
			} else {
				status.Set(err)
			}

			// Always record the signature and status
			if sig, ok := signature.(*protocol.RemoteSignature); ok {
				signature = sig.Signature
			}
			if err2 := batch.Transaction(signature.Hash()).Main().Put(&database.SigOrTxn{Signature: signature, Txid: delivery.Transaction.ID()}); err2 != nil {
				x.logger.Error("Failed to store signature", "error", err2)
			}
			if err2 := batch.Transaction(signature.Hash()).Status().Put(status); err2 != nil {
				x.logger.Error("Failed to store signature status", "error", err2)
			}

			if err != nil {
				if _, ok := err.(*errors.Error); ok {
					didFail = true
					block.State.MergeSignature(&ProcessSignatureState{})
					continue
				}
				return nil, nil, err
			}
			block.State.MergeSignature(s)
		}

		err = batch.Commit()
		if err != nil {
			return nil, nil, errors.UnknownError.WithFormat("commit batch: %w", err)
		}

		if didFail {
			return nil, nil, nil
		}
	}

	// Reload the transaction status to get changes made by processing signatures
	if x.globals.Active.ExecutorVersion.SignatureAnchoringEnabled() {
		status, err = block.Batch.Transaction(delivery.Transaction.GetHash()).Status().Get()
		if err != nil {
			return nil, nil, errors.UnknownError.WithFormat("load status: %w", err)
		}
	}

	if delivery.WasProducedInternally() || shouldProcessTransaction {
		// Process the transaction
		batch := block.Batch.Begin(true)
		defer batch.Discard()

		var state *chain.ProcessTransactionState
		status, state, err = x.ProcessTransaction(batch, delivery)
		if err != nil {
			return nil, nil, err
		}

		err = batch.Commit()
		if err != nil {
			return nil, nil, errors.UnknownError.WithFormat("commit batch: %w", err)
		}

		delivery.State.Merge(state)

		kv := []interface{}{
			"block", block.Index,
			"type", txnType,
			"code", status.Code,
			"txn-hash", logging.AsHex(delivery.Transaction.GetHash()).Slice(0, 4),
			"principal", delivery.Transaction.Header.Principal,
		}
		if status.Error != nil {
			kv = append(kv, "error", status.Error)
			if additional {
				x.logger.Info("Additional transaction failed", kv...)
			} else {
				x.logger.Info("Transaction failed", kv...)
			}
		} else if status.Pending() {
			if additional {
				x.logger.Debug("Additional transaction pending", kv...)
			} else {
				x.logger.Debug("Transaction pending", kv...)
			}
		} else {
			fn := x.logger.Debug
			switch txnType {
			case protocol.TransactionTypeDirectoryAnchor,
				protocol.TransactionTypeBlockValidatorAnchor:
				fn = x.logger.Info
				kv = append(kv, "module", "anchoring")
			}
			if additional {
				fn("Additional transaction succeeded", kv...)
			} else {
				fn("Transaction succeeded", kv...)
			}
		}

	} else if status.Code == 0 {
		status.Code = errors.Remote

		batch := block.Batch.Begin(true)
		defer batch.Discard()

		err = batch.Transaction(delivery.Transaction.GetHash()).Status().Put(status)
		if err != nil {
			return nil, nil, errors.UnknownError.Wrap(err)
		}
		if delivery.Transaction.Body.Type() != protocol.TransactionTypeRemote {
			err = batch.Transaction(delivery.Transaction.GetHash()).Main().Put(&database.SigOrTxn{Transaction: delivery.Transaction})
			if err != nil {
				return nil, nil, errors.UnknownError.Wrap(err)
			}
		}

		err = batch.Commit()
		if err != nil {
			return nil, nil, errors.UnknownError.WithFormat("commit batch: %w", err)
		}
	}

	err = x.ProcessRemoteSignatures(block, delivery)
	if err != nil {
		return nil, nil, err
	}

	block.State.MergeTransaction(&delivery.State)

	// Process synthetic transactions generated by the validator
	{
		batch := block.Batch.Begin(true)
		defer batch.Discard()

		err = x.ProduceSynthetic(batch, delivery.Transaction, delivery.State.ProducedTxns)
		if err != nil {
			return nil, nil, errors.UnknownError.Wrap(err)
		}

		err = batch.Commit()
		if err != nil {
			return nil, nil, errors.UnknownError.WithFormat("commit batch: %w", err)
		}
	}

	// Let the caller process additional transactions. It would be easier to do
	// this here, recursively, but it's possible that could cause a stack
	// overflow.
	return status, delivery.State.AdditionalTransactions, nil
}

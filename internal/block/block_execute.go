package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Block struct {
	BlockMeta
	State BlockState
	Batch *database.Batch
}

func (x *Executor) ExecuteEnvelope(block *Block, delivery *chain.Delivery) (*protocol.TransactionStatus, error) {
	if !delivery.Transaction.Body.Type().IsSystem() {
		x.logger.Debug("Executing transaction",
			"block", block.Index,
			"type", delivery.Transaction.Body.Type(),
			"txn-hash", logging.AsHex(delivery.Transaction.GetHash()).Slice(0, 4),
			"principal", delivery.Transaction.Header.Principal)
	}

	if delivery.Transaction.Body.Type() == protocol.TransactionTypeSystemWriteData {
		return nil, errors.Format(errors.StatusBadRequest, "a %v transaction cannot be submitted directly", protocol.TransactionTypeSystemWriteData)
	}

	status, additional, err := x.executeEnvelope(block, delivery)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	// Process additional transactions. This is intentionally non-recursive.
	for len(additional) > 0 {
		var next []*chain.Delivery
		for _, delivery := range additional {
			if !delivery.Transaction.Body.Type().IsSystem() {
				x.logger.Debug("Executing additional",
					"block", block.Index,
					"type", delivery.Transaction.Body.Type(),
					"txn-hash", logging.AsHex(delivery.Transaction.GetHash()).Slice(0, 4),
					"principal", delivery.Transaction.Header.Principal)
			}
			status, additional, err := x.executeEnvelope(block, delivery)
			if err != nil {
				return nil, errors.Wrap(errors.StatusUnknownError, err)
			}

			next = append(next, additional...)
			if err != nil {
				return nil, err
			}

			if status.Failed() {
				x.logger.Error("Additional transaction failed",
					"block", block.Index,
					"type", delivery.Transaction.Body.Type(),
					"txn-hash", logging.AsHex(delivery.Transaction.GetHash()).Slice(0, 4),
					"principal", delivery.Transaction.Header.Principal,
					"status", status.Code,
					"error", status.Error,
				)
			}
		}
		additional, next = next, nil
	}

	return status, nil
}

func (x *Executor) executeEnvelope(block *Block, delivery *chain.Delivery) (*protocol.TransactionStatus, []*chain.Delivery, error) {
	r := x.BlockTimers.Start(BlockTimerTypeExecuteEnvelope)
	defer x.BlockTimers.Stop(r)

	status, err := delivery.LoadTransaction(block.Batch)
	switch {
	case err == nil:
		// Ok

	case !errors.Is(err, errors.StatusDelivered):
		// Unknown error
		return nil, nil, errors.Wrap(errors.StatusUnknownError, err)

	default:
		// Transaction has already been delivered
		status := status.Copy()
		status.Code = errors.StatusDelivered
		return status, nil, nil
	}

	if delivery.Transaction.Body.Type().IsSynthetic() {
		err = delivery.LoadSyntheticMetadata(block.Batch, status)
		if err != nil {
			return nil, nil, errors.Wrap(errors.StatusUnknownError, err)
		}
	}

	// Process signatures
	shouldProcessTransaction := !delivery.Transaction.Body.Type().IsUser()
	{
		batch := block.Batch.Begin(true)
		defer batch.Discard()

		for _, signature := range delivery.Signatures {
			if !signature.Type().IsSystem() && signature.RoutingLocation().LocalTo(delivery.Transaction.Header.Principal) {
				shouldProcessTransaction = true
			}

			s, err := x.ProcessSignature(batch, delivery, signature)
			if err, ok := err.(*errors.Error); ok {
				status := new(protocol.TransactionStatus)
				status.Set(err)
				status.Result = new(protocol.EmptyResult)
				return status, nil, nil
			}
			if err != nil {
				return nil, nil, err
			}
			block.State.MergeSignature(s)
		}

		err = batch.Commit()
		if err != nil {
			return nil, nil, errors.Format(errors.StatusUnknownError, "commit batch: %w", err)
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
			return nil, nil, errors.Format(errors.StatusUnknownError, "commit batch: %w", err)
		}

		delivery.State.Merge(state)

		if typ := delivery.Transaction.Body.Type(); typ == protocol.TransactionTypeSystemGenesis || !typ.IsSystem() {
			kv := []interface{}{
				"block", block.Index,
				"type", delivery.Transaction.Body.Type(),
				"code", status.Code,
				"txn-hash", logging.AsHex(delivery.Transaction.GetHash()).Slice(0, 4),
				"principal", delivery.Transaction.Header.Principal,
			}
			if status.Error != nil {
				kv = append(kv, "error", status.Error)
				x.logger.Info("Transaction failed", kv...)
			} else if !delivery.Transaction.Body.Type().IsSystem() {
				x.logger.Debug("Transaction succeeded", kv...)
			}
		}

	} else {
		status = &protocol.TransactionStatus{Code: errors.StatusRemote}
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
			return nil, nil, errors.Wrap(errors.StatusUnknownError, err)
		}

		err = batch.Commit()
		if err != nil {
			return nil, nil, errors.Format(errors.StatusUnknownError, "commit batch: %w", err)
		}
	}

	// Let the caller process additional transactions. It would be easier to do
	// this here, recursively, but it's possible that could cause a stack
	// overflow.
	return status, delivery.State.AdditionalTransactions, nil
}

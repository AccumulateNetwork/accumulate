package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Block struct {
	BlockMeta
	State BlockState
	Batch *database.Batch
}

func (x *Executor) ExecuteEnvelopeSet(block *Block, deliveries []*execute.Delivery, captureError func(error, *execute.Delivery, *protocol.TransactionStatus)) []*protocol.TransactionStatus {
	results := make([]*protocol.TransactionStatus, len(deliveries))
	for i, delivery := range deliveries {
		status, err := x.ExecuteEnvelope(block, delivery)
		if status == nil {
			status = new(protocol.TransactionStatus)
		}
		results[i] = status

		// Wait until after ExecuteEnvelope, because the transaction may get
		// loaded by LoadTransaction
		status.TxID = delivery.Transaction.ID()

		if err != nil {
			status.Set(err)
			if captureError != nil {
				captureError(err, delivery, status)
			}
		}
	}

	return results
}

func (x *Executor) ExecuteEnvelope(block *Block, delivery *execute.Delivery) (*protocol.TransactionStatus, error) {

	if delivery.Transaction.Body.Type() == protocol.TransactionTypeSystemWriteData {
		return nil, errors.Format(errors.StatusBadRequest, "a %v transaction cannot be submitted directly", protocol.TransactionTypeSystemWriteData)
	}

	status, additional, err := x.executeEnvelope(block, delivery, false)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	// Record when the transaction is received
	if status.Received == 0 {
		status.Received = block.Index
		err = block.Batch.Transaction(delivery.Transaction.GetHash()).PutStatus(status)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknownError, err)
		}
	}

	// Process additional transactions. This is intentionally non-recursive.
	for len(additional) > 0 {
		var next []*execute.Delivery
		for _, delivery := range additional {
			_, additional, err := x.executeEnvelope(block, delivery, true)
			if err != nil {
				return nil, errors.Wrap(errors.StatusUnknownError, err)
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

func (x *Executor) executeEnvelope(block *Block, delivery *execute.Delivery, additional bool) (*protocol.TransactionStatus, []*execute.Delivery, error) {
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

	case !errors.Is(err, errors.StatusDelivered):
		// Unknown error
		return nil, nil, errors.Wrap(errors.StatusUnknownError, err)

	default:
		// Transaction has already been delivered
		status := status.Copy()
		status.Code = errors.StatusDelivered
		return status, nil, nil
	}

	err = delivery.LoadSyntheticMetadata(block.Batch, delivery.Transaction.Body.Type(), status)
	if err != nil {
		return nil, nil, errors.Wrap(errors.StatusUnknownError, err)
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

		var state *execute.ProcessTransactionState
		status, state, err = x.ProcessTransaction(batch, delivery)
		if err != nil {
			return nil, nil, err
		}

		err = batch.Commit()
		if err != nil {
			return nil, nil, errors.Format(errors.StatusUnknownError, "commit batch: %w", err)
		}

		delivery.State.Merge(state)

		kv := []interface{}{
			"block", block.Index,
			"type", delivery.Transaction.Body.Type(),
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
			switch delivery.Transaction.Body.Type() {
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
		status.Code = errors.StatusRemote
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

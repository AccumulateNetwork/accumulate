package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Block struct {
	BlockMeta
	State BlockState
	Batch *database.Batch
}

func (x *Executor) ExecuteEnvelope(block *Block, delivery *chain.Delivery) (*protocol.TransactionStatus, error) {
	if delivery.Transaction.Body.Type() == protocol.TransactionTypeSystemWriteData {
		return nil, errors.Format(errors.StatusBadRequest, "a %v transaction cannot be submitted directly", protocol.TransactionTypeSystemWriteData)
	}

	status, additional, err := x.executeEnvelope(block, delivery)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}

	// Process additional transactions. This is intentionally non-recursive.
	for len(additional) > 0 {
		var next []*chain.Delivery
		for _, delivery := range additional {
			status, additional, err := x.executeEnvelope(block, delivery)
			if err != nil {
				return nil, errors.Wrap(errors.StatusUnknown, err)
			}

			next = append(next, additional...)
			if err != nil {
				return nil, err
			}

			if status.Code != 0 && status.Code != protocol.ErrorCodeAlreadyDelivered.GetEnumValue() {
				var statusErr error
				if status.Error != nil {
					statusErr = status.Error
				} else {
					statusErr = protocol.Errorf(protocol.ErrorCode(status.Code), "%s", status.Message)
				}
				x.logger.Error("Additional transaction failed",
					"block", block.Index,
					"type", delivery.Transaction.Body.Type(),
					"pending", status.Pending,
					"delivered", status.Delivered,
					"remote", status.Remote,
					"txn-hash", logging.AsHex(delivery.Transaction.GetHash()).Slice(0, 4),
					"principal", delivery.Transaction.Header.Principal,
					"code", status.Code,
					"error", statusErr,
				)
			}
		}
		additional, next = next, nil
	}

	return status, nil
}

func (x *Executor) executeEnvelope(block *Block, delivery *chain.Delivery) (*protocol.TransactionStatus, []*chain.Delivery, error) {
	status, err := delivery.LoadTransaction(block.Batch)
	switch {
	case err == nil:
		// Ok

	case !errors.Is(err, errors.StatusDelivered):
		// Unknown error
		return nil, nil, errors.Wrap(errors.StatusUnknown, err)

	default:
		// Transaction has already been delivered
		status := status.Copy()
		status.Code = protocol.ErrorCodeAlreadyDelivered.GetEnumValue()
		return status, nil, nil
	}

	txType := delivery.Transaction.Body.Type()
	if txType.IsSynthetic() {
		err = delivery.LoadSyntheticMetadata(block.Batch, status)
		if err != nil {
			return nil, nil, errors.Wrap(errors.StatusUnknown, err)
		}
	}

	// Process signatures
	shouldProcessTransaction := !txType.IsUser()
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
				status.Code = protocol.ErrorCodeInvalidSignature.GetEnumValue()
				status.Message = err.Message
				status.Error = err
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
			return nil, nil, protocol.Errorf(protocol.ErrorCodeUnknownError, "commit batch: %w", err)
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
			return nil, nil, protocol.Errorf(protocol.ErrorCodeUnknownError, "commit batch: %w", err)
		}

		delivery.State.Merge(state)

		if !txType.IsSystem() {
			kv := []interface{}{
				"block", block.Index,
				"type", txType,
				"pending", status.Pending,
				"delivered", status.Delivered,
				"remote", status.Remote,
				"txn-hash", logging.AsHex(delivery.Transaction.GetHash()).Slice(0, 4),
				"principal", delivery.Transaction.Header.Principal,
			}
			if status.Code != 0 {
				kv = append(kv,
					"code", status.Code,
					"error", status.Message,
				)
				x.Logger.Info("Transaction failed", kv...)
			} else {
				x.Logger.Debug("Transaction succeeded", kv...)
			}
		}

	} else {
		status = &protocol.TransactionStatus{Remote: true}
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
			return nil, nil, protocol.NewError(protocol.ErrorCodeUnknownError, err)
		}

		err = batch.Commit()
		if err != nil {
			return nil, nil, protocol.Errorf(protocol.ErrorCodeUnknownError, "commit batch: %w", err)
		}
	}

	// Emit events for data chain update transactions (addressed to own node URL only)
	if status.Delivered && !x.isGenesis {
		switch v := status.Result.(type) {
		case *protocol.WriteDataResult:
			if v.AccountUrl.Authority == x.Network.NodeUrl().Authority && txType == protocol.TransactionTypeWriteData {
				body, ok := delivery.Transaction.Body.(*protocol.WriteData)
				if !ok {
					return nil, nil, errors.Format(errors.StatusInternalError, "invalid payload: want %T, got %T", new(protocol.WriteData), delivery.Transaction.Body)
				}
				x.eventBus.Publish(events.DidDataAccountUpdate{
					AccountUrl: v.AccountUrl,
					DataEntry:  &body.Entry,
				})
			}
		}
	}

	// Let the caller process additional transactions. It would be easier to do
	// this here, recursively, but it's possible that could cause a stack
	// overflow.
	return status, delivery.State.AdditionalTransactions, nil
}

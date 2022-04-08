package abci

import (
	"crypto/sha256"
	"errors"

	"github.com/getsentry/sentry-go"
	"github.com/tendermint/tendermint/libs/log"
	. "gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type executeFunc func(*protocol.Envelope) (protocol.TransactionResult, error)

func executeTransactions(logger log.Logger, execute executeFunc, raw []byte) ([]*protocol.Envelope, []*protocol.TransactionStatus, []byte, *protocol.Error) {
	hash := sha256.Sum256(raw)
	envelopes, err := transactions.UnmarshalAll(raw)
	if err != nil {
		sentry.CaptureException(err)
		logger.Info("Failed to unmarshal", "tx", logging.AsHex(hash), "error", err)
		return nil, nil, nil, &protocol.Error{Code: protocol.ErrorCodeEncodingError, Message: errors.New("Unable to decode transaction(s)")}
	}

	results := make([]*protocol.TransactionStatus, len(envelopes))
	for i, env := range envelopes {
		status := new(protocol.TransactionStatus)
		result, err := execute(env)
		if err != nil {
			sentry.CaptureException(err)
			if err, ok := err.(*protocol.Error); ok {
				status.Code = err.Code.GetEnumValue()
			} else {
				status.Code = protocol.ErrorCodeUnknownError.GetEnumValue()
			}
			status.Message = err.Error()
		}

		status.Result = result
		results[i] = status
	}

	// If the results can't be marshaled, provide no results but do not fail the
	// batch
	rset, err := (&protocol.TransactionResultSet{Results: results}).MarshalBinary()
	if err != nil {
		sentry.CaptureException(err)
		logger.Error("Unable to encode result", "error", err)
		return envelopes, results, nil, nil
	}

	return envelopes, results, rset, nil
}

func checkTx(exec *Executor, db *database.Database) executeFunc {
	return func(envelope *protocol.Envelope) (protocol.TransactionResult, error) {
		batch := db.Begin(false)
		defer batch.Discard()

		result, err := exec.ValidateEnvelope(batch, envelope)
		if err != nil {
			return nil, protocol.NewError(protocol.ErrorCodeUnknownError, err)
		}
		if result == nil {
			return new(protocol.EmptyResult), nil
		}
		return result, nil
	}
}

func deliverTx(exec *Executor, block *Block) executeFunc {
	return func(envelope *protocol.Envelope) (protocol.TransactionResult, error) {
		delivery, err := PrepareDelivery(block, envelope)
		if err != nil {
			return nil, err
		}

		status, err := exec.ExecuteEnvelope(block, delivery)
		if err != nil {
			return nil, err
		}

		return status.Result, nil
	}
}

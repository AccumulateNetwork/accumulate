package abci

import (
	"crypto/sha256"
	"errors"

	"github.com/getsentry/sentry-go"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type executeFunc func(*protocol.Envelope) (protocol.TransactionResult, *protocol.Error)

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
		typ := env.Transaction.Type()
		txid := env.GetTxHash()
		status := new(protocol.TransactionStatus)

		result, err := execute(env)
		if err != nil {
			sentry.CaptureException(err)
			logger.Info("Transaction failed",
				"type", env.Transaction.Type(),
				"txid", logging.AsHex(txid),
				"hash", logging.AsHex(hash),
				"error", err,
				"principal", env.Transaction.Header.Principal)
			status.Code = err.Code.ID()
			status.Message = err.Error()
		} else if !typ.IsInternal() && typ != protocol.TransactionTypeSyntheticAnchor {
			logger.Debug("Transaction succeeded",
				"type", typ,
				"txid", logging.AsHex(txid),
				"hash", logging.AsHex(hash))
		}

		status.Result = result
		results[i] = status
	}

	// If the results can't be marshaled, provide no results but do not fail the
	// batch
	var data []byte
	for _, r := range results {
		d, err := r.MarshalBinary()
		if err != nil {
			sentry.CaptureException(err)
			logger.Error("Unable to encode result", "error", err)
			return envelopes, results, nil, nil
		}
		data = append(data, d...)
	}

	return envelopes, results, data, nil
}

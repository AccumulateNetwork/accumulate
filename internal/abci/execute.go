package abci

//lint:file-ignore ST1001 Don't care

import (
	"crypto/sha256"

	"github.com/getsentry/sentry-go"
	"github.com/tendermint/tendermint/libs/log"
	. "gitlab.com/accumulatenetwork/accumulate/internal/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type executeFunc func([]*chain.Delivery) []*protocol.TransactionStatus

func executeTransactions(logger log.Logger, execute executeFunc, raw []byte) ([]*chain.Delivery, []*protocol.TransactionStatus, []byte, error) {
	hash := sha256.Sum256(raw)
	envelope := new(protocol.Envelope)
	err := envelope.UnmarshalBinary(raw)
	if err != nil {
		sentry.CaptureException(err)
		logger.Info("Failed to unmarshal", "tx", logging.AsHex(hash), "error", err)
		return nil, nil, nil, errors.Format(errors.StatusUnknownError, "decoding envelopes: %w", err)
	}

	deliveries, err := chain.NormalizeEnvelope(envelope)
	if err != nil {
		sentry.CaptureException(err)
		logger.Info("Failed to normalize envelope", "tx", logging.AsHex(hash), "error", err)
		return nil, nil, nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	results := execute(deliveries)

	// If the results can't be marshaled, provide no results but do not fail the
	// batch
	rset, err := (&protocol.TransactionResultSet{Results: results}).MarshalBinary()
	if err != nil {
		sentry.CaptureException(err)
		logger.Error("Unable to encode result", "error", err)
		return deliveries, results, nil, nil
	}

	return deliveries, results, rset, nil
}

func checkTx(exec *Executor, batch *database.Batch) executeFunc {
	return func(deliveries []*chain.Delivery) []*protocol.TransactionStatus {
		return exec.ValidateEnvelopeSet(batch, deliveries, func(err error, _ *chain.Delivery, _ *protocol.TransactionStatus) {
			sentry.CaptureException(err)
		})
	}
}

func deliverTx(exec *Executor, block *Block) executeFunc {
	return func(deliveries []*chain.Delivery) []*protocol.TransactionStatus {
		return exec.ExecuteEnvelopeSet(block, deliveries, func(err error, _ *chain.Delivery, _ *protocol.TransactionStatus) {
			sentry.CaptureException(err)
		})
	}
}

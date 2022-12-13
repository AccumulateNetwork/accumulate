// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package abci

//lint:file-ignore ST1001 Don't care

import (
	"crypto/sha256"

	"github.com/tendermint/tendermint/libs/log"
	. "gitlab.com/accumulatenetwork/accumulate/internal/core/v1/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/v1/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type executeFunc func([]*chain.Delivery) []*protocol.TransactionStatus

func executeTransactions(logger log.Logger, execute executeFunc, raw []byte) ([]*chain.Delivery, []*protocol.TransactionStatus, []byte, error) {
	hash := sha256.Sum256(raw)
	envelope := new(protocol.Envelope)
	err := envelope.UnmarshalBinary(raw)
	if err != nil {
		logger.Info("Failed to unmarshal", "tx", logging.AsHex(hash), "error", err)
		return nil, nil, nil, errors.UnknownError.WithFormat("decoding envelopes: %w", err)
	}

	deliveries, err := chain.NormalizeEnvelope(envelope)
	if err != nil {
		logger.Info("Failed to normalize envelope", "tx", logging.AsHex(hash), "error", err)
		return nil, nil, nil, errors.UnknownError.Wrap(err)
	}

	results := execute(deliveries)

	// If the results can't be marshaled, provide no results but do not fail the
	// batch
	rset, err := (&protocol.TransactionResultSet{Results: results}).MarshalBinary()
	if err != nil {
		logger.Error("Unable to encode result", "error", err)
		return deliveries, results, nil, nil
	}

	return deliveries, results, rset, nil
}

func checkTx(exec *Executor, batch *database.Batch) executeFunc {
	return func(deliveries []*chain.Delivery) []*protocol.TransactionStatus {
		return exec.ValidateEnvelopeSet(batch, deliveries, nil)
	}
}

func deliverTx(exec *Executor, block *Block) executeFunc {
	return func(deliveries []*chain.Delivery) []*protocol.TransactionStatus {
		return exec.ExecuteEnvelopeSet(block, deliveries, nil)
	}
}

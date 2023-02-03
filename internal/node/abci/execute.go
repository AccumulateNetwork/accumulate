// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package abci

//lint:file-ignore ST1001 Don't care

import (
	"crypto/sha256"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type executeFunc func([]messaging.Message) ([]*protocol.TransactionStatus, error)

func executeTransactions(logger log.Logger, execute executeFunc, raw []byte) ([]messaging.Message, []*protocol.TransactionStatus, []byte, error) {
	hash := sha256.Sum256(raw)
	envelope := new(messaging.Envelope)
	err := envelope.UnmarshalBinary(raw)
	if err != nil {
		logger.Info("Failed to unmarshal", "tx", logging.AsHex(hash), "error", err)
		return nil, nil, nil, errors.UnknownError.WithFormat("decoding envelopes: %w", err)
	}

	deliveries, err := envelope.Normalize()
	if err != nil {
		logger.Info("Failed to normalize envelope", "tx", logging.AsHex(hash), "error", err)
		return nil, nil, nil, errors.UnknownError.Wrap(err)
	}

	results, err := execute(deliveries)
	if err != nil {
		logger.Info("Failed to execute messages", "tx", logging.AsHex(hash), "error", err)
		return nil, nil, nil, errors.UnknownError.Wrap(err)
	}

	// If the results can't be marshaled, provide no results but do not fail the
	// batch
	rset, err := (&protocol.TransactionResultSet{Results: results}).MarshalBinary()
	if err != nil {
		logger.Error("Unable to encode result", "error", err)
		return deliveries, results, nil, nil
	}

	return deliveries, results, rset, nil
}

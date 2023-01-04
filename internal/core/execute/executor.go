// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	abci "github.com/tendermint/tendermint/abci/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Executor interface {
	EnableTimers()
	StoreBlockTimers(ds *logging.DataSet)
	SetBackgroundTaskManager(f func(func()))

	LoadStateRoot(*database.Batch) ([]byte, error)
	RestoreSnapshot(database.Beginner, ioutil2.SectionReader) error
	InitChainValidators(initVal []abci.ValidatorUpdate) (additional [][]byte, err error)

	ValidateEnvelope(*database.Batch, messaging.Message) (*protocol.TransactionStatus, error)
	BeginBlock(*block.Block) error
	ExecuteEnvelope(*block.Block, messaging.Message) (*protocol.TransactionStatus, error)
	EndBlock(*block.Block) error
}

func ValidateEnvelopeSet(x Executor, batch *database.Batch, deliveries []messaging.Message) []*protocol.TransactionStatus {
	results := make([]*protocol.TransactionStatus, len(deliveries))
	for i, delivery := range deliveries {
		status, err := x.ValidateEnvelope(batch, delivery)
		if status == nil {
			status = new(protocol.TransactionStatus)
		}
		results[i] = status

		if err != nil {
			status.Set(err)
		}
	}

	return results
}

func ExecuteEnvelopeSet(x Executor, b *block.Block, deliveries []messaging.Message) []*protocol.TransactionStatus {
	results := make([]*protocol.TransactionStatus, len(deliveries))
	for i, delivery := range deliveries {
		status, err := x.ExecuteEnvelope(b, delivery)
		if status == nil {
			status = new(protocol.TransactionStatus)
		}
		results[i] = status

		if err != nil {
			status.Set(err)
		}
	}

	return results
}

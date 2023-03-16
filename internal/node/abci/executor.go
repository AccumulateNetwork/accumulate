// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package abci

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Executor interface {
	EnableTimers()
	StoreBlockTimers(ds *logging.DataSet)

	LastBlock() (uint64, [32]byte, error)
	Restore(snapshot ioutil2.SectionReader, validators []*ValidatorUpdate) (additional []*ValidatorUpdate, err error)

	Validate(messages []messaging.Message, recheck bool) ([]*protocol.TransactionStatus, error)
	Begin(execute.BlockParams) (execute.Block, error)
}

type ValidatorUpdate struct {
	Type      protocol.SignatureType
	PublicKey []byte
	Power     int64
}

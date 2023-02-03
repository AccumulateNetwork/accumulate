// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SystemWriteData struct{}

func (SystemWriteData) Type() protocol.TransactionType {
	return protocol.TransactionTypeSystemWriteData
}

func (SystemWriteData) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (SystemWriteData{}).Validate(st, tx)
}

func (SystemWriteData) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SystemWriteData)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SystemWriteData), tx.Transaction.Body)
	}

	err := validateDataEntry(st, body.Entry)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	if partition, ok := protocol.ParsePartitionUrl(st.OriginUrl); !ok {
		return nil, errors.BadRequest.WithFormat("invalid principal: %v is not a system account", st.OriginUrl)
	} else if partition != st.PartitionId {
		return nil, errors.BadRequest.WithFormat("invalid principal: %v belongs to the wrong partition", st.OriginUrl)
	}

	return executeWriteFullDataAccount(st, body.Entry, false, body.WriteToState)
}

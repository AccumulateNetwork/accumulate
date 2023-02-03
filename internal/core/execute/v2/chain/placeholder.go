// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Placeholder struct{}

func (Placeholder) Type() protocol.TransactionType {
	return protocol.TransactionTypePlaceholder
}

func (Placeholder) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return nil, nil
}

func (Placeholder) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return nil, nil
}

// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	registerSimpleExec[BlockAnchor](&executors, messaging.MessageTypeBlockAnchor)
}

type BlockAnchor struct{}

func (x BlockAnchor) Validate(batch *Block, ctx *MessageContext) (*protocol.TransactionStatus, error) {
	return nil, nil
}

func (x BlockAnchor) Process(batch *Block, ctx *MessageContext) (_ *protocol.TransactionStatus, err error) {
	return nil, nil
}

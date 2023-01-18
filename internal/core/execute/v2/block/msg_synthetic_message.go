// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	messageExecutors = append(messageExecutors, func(ExecutorOptions) MessageExecutor { return SyntheticMessage{} })
}

// SyntheticMessage marks a synthetic transaction for processing.
type SyntheticMessage struct{}

func (SyntheticMessage) Type() messaging.MessageType { return internal.MessageTypeSyntheticMessage }

func (SyntheticMessage) Process(b *bundle, batch *database.Batch, msg messaging.Message) (*protocol.TransactionStatus, error) {
	synth, ok := msg.(*internal.SyntheticMessage)
	if !ok {
		return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", internal.MessageTypeSyntheticMessage, msg.Type())
	}
	b.transactionsToProcess.Add(synth.TxID.Hash())
	return nil, nil
}

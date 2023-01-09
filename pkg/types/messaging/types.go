// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package messaging

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum --package messaging enums.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package messaging messages.yml
//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package messaging --out unions_gen.go --language go-union messages.yml

// MessageType is the type of a [Message].
type MessageType int

// A Message is signature, transaction, or other message that may be sent to
// Accumulate to be processed.
type Message interface {
	encoding.UnionValue

	// ID is the ID of the message.
	ID() *url.TxID

	// Type is the type of the message.
	Type() MessageType
}

func (m *UserTransaction) ID() *url.TxID { return m.Transaction.ID() }

func (m *UserSignature) ID() *url.TxID {
	switch sig := m.Signature.(type) {
	case *protocol.ReceiptSignature:
		return sig.SourceNetwork.WithTxID(sig.TransactionHash)
	case *protocol.InternalSignature:
		return protocol.UnknownUrl().WithTxID(sig.TransactionHash)
	default:
		return sig.RoutingLocation().WithTxID(sig.GetTransactionHash())
	}
}

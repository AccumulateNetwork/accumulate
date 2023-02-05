// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package internal

// This file defines Message types that are used internally but must not be
// transmitted over the network.

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const (
	MessageTypeInternal messaging.MessageType = 0xFFFF_0000 + iota

	// MessageTypeNetworkUpdate is an update to a network account that has been
	// pushed from the DN via an anchor.
	MessageTypeNetworkUpdate

	// MessageTypeForwardedMessage is a message forwarded from another
	// partition.
	MessageTypeForwardedMessage

	// MessageTypeTransactionIsReady indicates that a transaction is ready to be
	// executed.
	MessageTypeTransactionIsReady
)

// NetworkUpdate is an update to a network account that has been pushed from the
// DN via an anchor.
type NetworkUpdate struct {
	internalMessage
	Cause   [32]byte
	Account *url.URL
	Body    protocol.TransactionBody
}

// ForwardedMessage is a message forwarded from another partition.
type ForwardedMessage struct {
	internalMessage
	Message messaging.Message
}

// TransactionIsReady indicates that ta transaction is ready to be executed.
type TransactionIsReady struct {
	internalMessage
	TxID *url.TxID
}

func (m *NetworkUpdate) Type() messaging.MessageType { return MessageTypeNetworkUpdate }
func (m *NetworkUpdate) ID() *url.TxID               { return m.Account.WithTxID(m.Cause) }
func (m *NetworkUpdate) CopyAsInterface() any        { return m }

func (m *ForwardedMessage) Type() messaging.MessageType { return MessageTypeForwardedMessage }
func (m *ForwardedMessage) ID() *url.TxID               { return m.Message.ID() }
func (m *ForwardedMessage) CopyAsInterface() any        { return m }

func (m *TransactionIsReady) Type() messaging.MessageType { return MessageTypeTransactionIsReady }
func (m *TransactionIsReady) ID() *url.TxID               { return m.TxID }
func (m *TransactionIsReady) CopyAsInterface() any        { return m }

// internalMessage can be embedded in another type to implement an internal
// [messaging.Message]. The message is internal in that it cannot be marshalled,
// thus cannot be sent across the network to another partition, and thus can
// only be produced locally within a partition and cannot come from an incoming
// envelope.
type internalMessage struct{}

// Hash panics.
func (internalMessage) Hash() [32]byte {
	panic("this is an internal message that must not be marshalled")
}

// MarshalBinary returns an error.
func (internalMessage) MarshalBinary() (data []byte, err error) {
	return nil, errors.InternalError.WithFormat("this is an internal message that must not be marshalled")
}

// UnmarshalBinary returns an error.
func (internalMessage) UnmarshalBinary(data []byte) error {
	return errors.InternalError.WithFormat("this is an internal message that must not be marshalled")
}

// UnmarshalFieldsFrom returns an error.
func (internalMessage) UnmarshalFieldsFrom(*encoding.Reader) error {
	return errors.InternalError.WithFormat("this is an internal message that must not be marshalled")
}

// UnmarshalBinaryFrom returns an error.
func (internalMessage) UnmarshalBinaryFrom(io.Reader) error {
	return errors.InternalError.WithFormat("this is an internal message that must not be marshalled")
}

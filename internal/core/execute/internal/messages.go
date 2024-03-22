// Copyright 2024 The Accumulate Authors
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

	// MessageTypeMessageIsReady indicates that a message is ready to be
	// processed.
	MessageTypeMessageIsReady

	// MessageTypePseudoSynthetic wraps produced messages between accounts in
	// the same domain that would otherwise be synthetic.
	MessageTypePseudoSynthetic

	// MessageTypeExpiredTransaction expires a pending transaction.
	MessageTypeExpiredTransaction

	// MessageTypeNetworkMaintenanceOp executes a network maintenance operation.
	MessageTypeNetworkMaintenanceOp
)

// NetworkUpdate is an update to a network account that has been pushed from the
// DN via an anchor.
type NetworkUpdate struct {
	internalMessage
	Cause   [32]byte
	Account *url.URL
	Body    protocol.TransactionBody
}

// MessageIsReady indicates that the transaction is ready to be executed.
type MessageIsReady struct {
	internalMessage
	TxID *url.TxID
}

// PseudoSynthetic wraps produced messages between accounts in the same domain
// that would otherwise be synthetic.
type PseudoSynthetic struct {
	internalMessage
	Message messaging.Message
}

// ExpiredTransaction expires a pending transaction.
type ExpiredTransaction struct {
	internalMessage
	TxID *url.TxID
}

// NetworkMaintenanceOp expires a network maintenance operation.
type NetworkMaintenanceOp struct {
	internalMessage
	Cause     *url.TxID
	Operation protocol.NetworkMaintenanceOperation
}

func (m *NetworkUpdate) Type() messaging.MessageType { return MessageTypeNetworkUpdate }
func (m *NetworkUpdate) ID() *url.TxID               { return m.Account.WithTxID(m.Cause) }
func (m *NetworkUpdate) CopyAsInterface() any        { return m }

func (m *MessageIsReady) Type() messaging.MessageType { return MessageTypeMessageIsReady }
func (m *MessageIsReady) ID() *url.TxID               { return m.TxID }
func (m *MessageIsReady) CopyAsInterface() any        { return m }

func (m *PseudoSynthetic) Type() messaging.MessageType { return MessageTypePseudoSynthetic }
func (m *PseudoSynthetic) ID() *url.TxID               { return m.Message.ID() }
func (m *PseudoSynthetic) CopyAsInterface() any        { return m }
func (m *PseudoSynthetic) Unwrap() messaging.Message   { return m.Message }

func (m *ExpiredTransaction) Type() messaging.MessageType { return MessageTypeExpiredTransaction }
func (m *ExpiredTransaction) ID() *url.TxID               { return m.TxID }
func (m *ExpiredTransaction) CopyAsInterface() any        { return m }
func (m *ExpiredTransaction) Hash() [32]byte              { return m.TxID.Hash() }

func (m *NetworkMaintenanceOp) Type() messaging.MessageType { return MessageTypeNetworkMaintenanceOp }
func (m *NetworkMaintenanceOp) ID() *url.TxID               { return m.Operation.ID() }
func (m *NetworkMaintenanceOp) CopyAsInterface() any        { return m }
func (m *NetworkMaintenanceOp) Hash() [32]byte              { return m.ID().Hash() }

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

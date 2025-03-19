// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package interfaces

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/model"
)

// Message is the interface that all messages must implement
type Message interface {
	Type() uint32
	CopyAsInterface() interface{}
	MarshalBinary() ([]byte, error)
	UnmarshalBinary([]byte) error
}

// Message type constants
const (
	TypeErrorResponse uint32 = iota + 1
	TypePrivateSequenceRequest
	TypePrivateSequenceResponse
)

// ErrorMessage is the interface for error responses
type ErrorMessage interface {
	Message
	GetError() *model.Error
}

// SequenceMessage is the interface for sequence messages
type SequenceMessage interface {
	Message
	GetOptions() *model.SequenceOptions
}

// RecordMessage is the interface for messages containing records
type RecordMessage interface {
	Message
	GetRecord() *model.MessageRecord
}

// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package messaging

// Message defines the interface for all messages
type Message interface {
	Type() uint32
	MarshalBinary() ([]byte, error)
	UnmarshalBinary([]byte) error
}

// MessageRecord represents a record containing a message
type MessageRecord struct {
	Message Message
}

// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package interfaces

import (
	"context"
	"io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/model"
)

// Message defines the interface for messages that can be sent over the network
type Message interface {
	Type() uint32
	CopyAsInterface() interface{}
	MarshalBinary() ([]byte, error)
	UnmarshalBinary([]byte) error
	UnmarshalBinaryFrom(io.Reader) error
}

// Transport defines the interface for message transport
type Transport interface {
	RoundTrip(ctx context.Context, requests []Message, callback ResponseCallback) error
}

// ResponseCallback is called for each response received
type ResponseCallback func(response Message) error

// ErrorResponse represents an error response
type ErrorResponse struct {
	model.Error
}

func (e *ErrorResponse) Type() uint32                    { return TypeErrorResponse }
func (e *ErrorResponse) CopyAsInterface() interface{}    { return &ErrorResponse{Error: e.Error} }
func (e *ErrorResponse) MarshalBinary() ([]byte, error) { return e.Error.MarshalBinary() }
func (e *ErrorResponse) UnmarshalBinary(data []byte) error {
	return e.Error.UnmarshalBinary(data)
}
func (e *ErrorResponse) UnmarshalBinaryFrom(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return e.UnmarshalBinary(data)
}

// PrivateSequenceRequest represents a request for a sequence of messages
type PrivateSequenceRequest struct {
	model.SequenceOptions
}

func (r *PrivateSequenceRequest) Type() uint32                    { return TypePrivateSequenceRequest }
func (r *PrivateSequenceRequest) CopyAsInterface() interface{}    { return &PrivateSequenceRequest{SequenceOptions: r.SequenceOptions} }
func (r *PrivateSequenceRequest) MarshalBinary() ([]byte, error) { return r.SequenceOptions.MarshalBinary() }
func (r *PrivateSequenceRequest) UnmarshalBinary(data []byte) error {
	return r.SequenceOptions.UnmarshalBinary(data)
}
func (r *PrivateSequenceRequest) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}

// PrivateSequenceResponse represents a response containing a sequence of messages
type PrivateSequenceResponse struct {
	Record *model.MessageRecord
}

func (r *PrivateSequenceResponse) Type() uint32                    { return TypePrivateSequenceResponse }
func (r *PrivateSequenceResponse) CopyAsInterface() interface{}    { return &PrivateSequenceResponse{Record: r.Record} }
func (r *PrivateSequenceResponse) MarshalBinary() ([]byte, error) { return r.Record.MarshalBinary() }
func (r *PrivateSequenceResponse) UnmarshalBinary(data []byte) error {
	return r.Record.UnmarshalBinary(data)
}
func (r *PrivateSequenceResponse) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(data)
}

// Message type constants
const (
	TypeErrorResponse uint32 = iota + 1
	TypePrivateSequenceRequest
	TypePrivateSequenceResponse
)

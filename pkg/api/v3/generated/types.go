// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package generated

import (
	"encoding/json"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/interfaces"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/model"
)

// Type constants for message types
const (
	TypeErrorResponse           uint32 = 1
	TypePrivateSequenceRequest uint32 = 2
	TypePrivateSequenceResponse uint32 = 3
)

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error *model.Error
}

func (r *ErrorResponse) Type() uint32 { return TypeErrorResponse }

func (r *ErrorResponse) GetError() *model.Error { return r.Error }

func (r *ErrorResponse) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &ErrorResponse{Error: r.Error.Copy()}
}

func (r *ErrorResponse) MarshalBinary() ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	return json.Marshal(r.Error)
}

func (r *ErrorResponse) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	r.Error = new(model.Error)
	return json.Unmarshal(data, r.Error)
}

// PrivateSequenceRequest represents a request for a sequence of messages
type PrivateSequenceRequest struct {
	Options *model.SequenceOptions
}

func (r *PrivateSequenceRequest) Type() uint32 { return TypePrivateSequenceRequest }

func (r *PrivateSequenceRequest) GetOptions() *model.SequenceOptions { return r.Options }

func (r *PrivateSequenceRequest) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &PrivateSequenceRequest{Options: r.Options.Copy()}
}

func (r *PrivateSequenceRequest) MarshalBinary() ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	return json.Marshal(r.Options)
}

func (r *PrivateSequenceRequest) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	r.Options = new(model.SequenceOptions)
	return json.Unmarshal(data, r.Options)
}

// PrivateSequenceResponse represents a response containing a sequence of messages
type PrivateSequenceResponse struct {
	Record *model.MessageRecord
}

func (r *PrivateSequenceResponse) Type() uint32 { return TypePrivateSequenceResponse }

func (r *PrivateSequenceResponse) GetRecord() *model.MessageRecord { return r.Record }

func (r *PrivateSequenceResponse) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &PrivateSequenceResponse{Record: r.Record.Copy()}
}

func (r *PrivateSequenceResponse) MarshalBinary() ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	return json.Marshal(r.Record)
}

func (r *PrivateSequenceResponse) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	r.Record = new(model.MessageRecord)
	return json.Unmarshal(data, r.Record)
}

// Ensure message types satisfy interfaces
var (
	_ interfaces.Message = (*ErrorResponse)(nil)
	_ interfaces.ErrorMessage = (*ErrorResponse)(nil)
	_ interfaces.Message = (*PrivateSequenceRequest)(nil)
	_ interfaces.SequenceMessage = (*PrivateSequenceRequest)(nil)
	_ interfaces.Message = (*PrivateSequenceResponse)(nil)
	_ interfaces.RecordMessage = (*PrivateSequenceResponse)(nil)
)

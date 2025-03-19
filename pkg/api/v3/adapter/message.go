// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package adapter

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/generated"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/interfaces"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/model"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// ErrorResponseAdapter adapts generated.ErrorResponse
type ErrorResponseAdapter struct {
	*generated.ErrorResponse
}

func NewErrorResponse(err *model.Error) *ErrorResponseAdapter {
	return &ErrorResponseAdapter{&generated.ErrorResponse{Error: err}}
}

func (r *ErrorResponseAdapter) Type() uint32 { return interfaces.TypeErrorResponse }
func (r *ErrorResponseAdapter) GetError() *model.Error { return r.Error }
func (r *ErrorResponseAdapter) GetEnumValue() uint64 { return uint64(r.Type()) }
func (r *ErrorResponseAdapter) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &ErrorResponseAdapter{&generated.ErrorResponse{Error: r.Error.Copy()}}
}

// PrivateSequenceRequestAdapter adapts generated.PrivateSequenceRequest
type PrivateSequenceRequestAdapter struct {
	*generated.PrivateSequenceRequest
	destination *url.URL
}

func NewPrivateSequenceRequest(options *model.SequenceOptions) *PrivateSequenceRequestAdapter {
	return &PrivateSequenceRequestAdapter{
		PrivateSequenceRequest: &generated.PrivateSequenceRequest{Options: options},
	}
}

func (r *PrivateSequenceRequestAdapter) Type() uint32 { return interfaces.TypePrivateSequenceRequest }
func (r *PrivateSequenceRequestAdapter) GetDestination() *url.URL { return r.destination }
func (r *PrivateSequenceRequestAdapter) SetDestination(u *url.URL) { r.destination = u }
func (r *PrivateSequenceRequestAdapter) GetOptions() *model.SequenceOptions { return r.Options }
func (r *PrivateSequenceRequestAdapter) GetEnumValue() uint64 { return uint64(r.Type()) }
func (r *PrivateSequenceRequestAdapter) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &PrivateSequenceRequestAdapter{
		PrivateSequenceRequest: &generated.PrivateSequenceRequest{Options: r.Options.Copy()},
		destination:            r.destination,
	}
}

// PrivateSequenceResponseAdapter adapts generated.PrivateSequenceResponse
type PrivateSequenceResponseAdapter struct {
	*generated.PrivateSequenceResponse
}

func NewPrivateSequenceResponse(record *model.MessageRecord) *PrivateSequenceResponseAdapter {
	return &PrivateSequenceResponseAdapter{&generated.PrivateSequenceResponse{Record: record}}
}

func (r *PrivateSequenceResponseAdapter) Type() uint32 { return interfaces.TypePrivateSequenceResponse }
func (r *PrivateSequenceResponseAdapter) GetRecord() *model.MessageRecord { return r.Record }
func (r *PrivateSequenceResponseAdapter) GetEnumValue() uint64 { return uint64(r.Type()) }
func (r *PrivateSequenceResponseAdapter) CopyAsInterface() interface{} {
	if r == nil {
		return nil
	}
	return &PrivateSequenceResponseAdapter{&generated.PrivateSequenceResponse{Record: r.Record.Copy()}}
}

// Ensure adapter types implement required interfaces
var (
	_ interfaces.Message = (*ErrorResponseAdapter)(nil)
	_ interfaces.Message = (*PrivateSequenceRequestAdapter)(nil)
	_ interfaces.Message = (*PrivateSequenceResponseAdapter)(nil)
	_ encoding.EnumValueGetter = (*ErrorResponseAdapter)(nil)
	_ encoding.EnumValueGetter = (*PrivateSequenceRequestAdapter)(nil)
	_ encoding.EnumValueGetter = (*PrivateSequenceResponseAdapter)(nil)
)

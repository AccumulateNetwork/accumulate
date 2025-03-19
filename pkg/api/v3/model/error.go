// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package model

import (
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// Error represents an error in the model
type Error struct {
	Code    uint32
	Message string
}

// Error returns the error message
func (e *Error) Error() string {
	return e.Message
}

// ToError converts the model error to a standard error
func (e *Error) ToError() error {
	if e == nil {
		return nil
	}
	return fmt.Errorf("%s (code: %d)", e.Message, e.Code)
}

// FromError converts a standard error to a model error
func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return e
	}
	if e, ok := err.(*errors.Error); ok {
		return &Error{
			Code:    uint32(e.Code),
			Message: e.Error(),
		}
	}
	return &Error{
		Code:    0,
		Message: err.Error(),
	}
}

// Copy creates a deep copy of the error
func (e *Error) Copy() *Error {
	if e == nil {
		return nil
	}
	return &Error{
		Code:    e.Code,
		Message: e.Message,
	}
}

// MarshalBinary implements encoding.BinaryMarshaler
func (e *Error) MarshalBinary() ([]byte, error) {
	if e == nil {
		return nil, nil
	}
	// Simple encoding: [4 bytes code][message]
	data := make([]byte, 4+len(e.Message))
	data[0] = byte(e.Code >> 24)
	data[1] = byte(e.Code >> 16)
	data[2] = byte(e.Code >> 8)
	data[3] = byte(e.Code)
	copy(data[4:], []byte(e.Message))
	return data, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler
func (e *Error) UnmarshalBinary(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("invalid error data")
	}
	e.Code = uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
	e.Message = string(data[4:])
	return nil
}

// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"encoding"
	"io"
)

type Error struct {
	E error
}

func (e Error) Error() string { return e.E.Error() }
func (e Error) Unwrap() error { return e.E }

type EnumValueGetter interface {
	GetEnumValue() uint64
}

type EnumValueSetter interface {
	SetEnumValue(uint64) bool
}

type BinaryValue interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
	CopyAsInterface() interface{}
	UnmarshalBinaryFrom(io.Reader) error
}

type UnionValue interface {
	BinaryValue
	UnmarshalFieldsFrom(reader *Reader) error
}
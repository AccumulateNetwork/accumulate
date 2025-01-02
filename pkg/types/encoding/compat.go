// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"fmt"

	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
	"gitlab.com/accumulatenetwork/core/schema/pkg/json"
	"gitlab.com/accumulatenetwork/core/schema/pkg/widget"
)

// EnumWidget is a [widget.Widget] for [EnumValueGetter] and [EnumValueSetter] types.
type EnumWidget[V enumSet[U], U enumGet] struct{}

func EnumWidgetFor[U enumGet, V enumSet[U]]() widget.Widget[V] {
	return EnumWidget[V, U]{}
}

// Enum types are assumed to be integers and thus are treated as such.

func (EnumWidget[V, U]) IsNil(v V) bool    { return false }
func (EnumWidget[V, U]) Empty(v V) bool    { return (*v).GetEnumValue() == 0 }
func (EnumWidget[V, U]) CopyTo(dst, src V) { *dst = *src }
func (EnumWidget[V, U]) Equal(a, b V) bool { return *a == *b }

// Enum types are expected to implement JSON un/marshalling.

func (EnumWidget[V, U]) MarshalJSON(e *json.Encoder, v V) error   { return e.Encode(v) }
func (EnumWidget[V, U]) UnmarshalJSON(d *json.Decoder, v V) error { return d.Decode(v) }

func (EnumWidget[V, U]) MarshalBinary(e *binary.Encoder, v V) error {
	return e.EncodeUint((*v).GetEnumValue())
}

func (EnumWidget[V, U]) UnmarshalBinary(d *binary.Decoder, v V) error {
	u, err := d.DecodeUint()
	if err != nil {
		return err
	}
	if !v.SetEnumValue(u) {
		return fmt.Errorf("%d is not a valid value of %T", u, *v)
	}
	return nil
}

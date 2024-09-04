// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"time"

	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
	"gitlab.com/accumulatenetwork/core/schema/pkg/json"
	"gitlab.com/accumulatenetwork/core/schema/pkg/widget"
)

func TimeWidget() widget.Widget[*time.Time] { return timeWidget{} }

type timeWidget struct{}

func (timeWidget) IsNil(v *time.Time) bool                           { return false }
func (timeWidget) Empty(v *time.Time) bool                           { return *v == time.Time{} }
func (timeWidget) CopyTo(dst, src *time.Time)                        { *dst = *src }
func (timeWidget) Equal(a, b *time.Time) bool                        { return (*a).Equal(*b) }
func (timeWidget) MarshalJSON(e *json.Encoder, v *time.Time) error   { return e.Encode(v) }
func (timeWidget) UnmarshalJSON(d *json.Decoder, v *time.Time) error { return d.Decode(v) }

func (timeWidget) MarshalBinary(e *binary.Encoder, v *time.Time) error {
	return (*Time)(v).MarshalBinaryV2(e)
}

func (timeWidget) UnmarshalBinary(d *binary.Decoder, v *time.Time) error {
	return (*Time)(v).UnmarshalBinaryV2(d)
}

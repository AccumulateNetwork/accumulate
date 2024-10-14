// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding

import (
	"errors"
	"strings"
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
	return e.EncodeInt(v.UTC().Unix())
}

func (timeWidget) UnmarshalBinary(d *binary.Decoder, v *time.Time) error {
	u, err := d.DecodeInt()
	if err != nil {
		return err
	}
	*v = time.Unix(u, 0).UTC()
	return nil
}

type Duration time.Duration

func (u Duration) Get() time.Duration   { return time.Duration(u) }
func (u *Duration) Set(v time.Duration) { *u = Duration(v) }

func (u Duration) String() string         { return (time.Duration)(u).String() }
func (u Duration) Copy() *Duration        { return &u }
func (u Duration) Equal(v *Duration) bool { return strings.EqualFold(u.String(), v.String()) }

func (u Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(u)
}

func (u *Duration) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, u)
}

func (u Duration) MarshalJSONV2(e *json.Encoder) error {
	return e.Encode(DurationToJSON(u.Get()))
}

func (u *Duration) UnmarshalJSONV2(d *json.Decoder) error {
	var v any
	err := d.Decode(&v)
	if err != nil {
		return err
	}
	w, err := DurationFromJSON(v)
	if err != nil {
		return err
	}
	u.Set(w)
	return nil
}

func (u Duration) MarshalBinary() ([]byte, error) {
	return binary.Marshal(u)
}

func (u *Duration) UnmarshalBinary(b []byte) error {
	return binary.Unmarshal(b, u)
}

func (u Duration) MarshalBinaryV2(e *binary.Encoder) error {
	sec, ns := SplitDuration(u.Get())
	return errors.Join(
		e.EncodeUint(sec),
		e.NoField(),
		e.EncodeUint(ns),
	)
}

func (u *Duration) UnmarshalBinaryV2(d *binary.Decoder) error {
	sec, e1 := d.DecodeUint()
	e2 := d.NoField()
	ns, e3 := d.DecodeUint()
	u.Set(time.Duration(sec)*time.Second + time.Duration(ns)*time.Nanosecond)
	return errors.Join(e1, e2, e3)
}

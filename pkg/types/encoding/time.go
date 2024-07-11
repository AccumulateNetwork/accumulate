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
)

type Time time.Time

func (u Time) Get() time.Time   { return time.Time(u) }
func (u *Time) Set(v time.Time) { *u = Time(v) }

func (u *Time) String() string     { return u.Get().String() }
func (u *Time) Copy() *Time        { return u }
func (u *Time) Equal(v *Time) bool { return strings.EqualFold(u.String(), v.String()) }

func (u *Time) MarshalJSON() ([]byte, error) {
	return json.Marshal(u)
}

func (u *Time) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, u)
}

func (u *Time) MarshalJSONV2(e *json.Encoder) error {
	return e.Encode(u.Get())
}

func (u *Time) UnmarshalJSONV2(d *json.Decoder) error {
	return d.Decode((*time.Time)(u))
}

func (u *Time) MarshalBinary() ([]byte, error) {
	return binary.Marshal(u)
}

func (u *Time) UnmarshalBinary(b []byte) error {
	return binary.Unmarshal(b, u)
}

func (u *Time) MarshalBinaryV2(e *binary.Encoder) error {
	return e.EncodeInt(u.Get().UTC().Unix())
}

func (u *Time) UnmarshalBinaryV2(d *binary.Decoder) error {
	v, err := d.DecodeInt()
	if err != nil {
		return err
	}
	u.Set(time.Unix(v, 0).UTC())
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

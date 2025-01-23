// Copyright 2025 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package indexing

import (
	"bytes"
	"reflect"

	"gitlab.com/accumulatenetwork/core/schema/pkg/binary"
	"gitlab.com/accumulatenetwork/core/schema/pkg/json"
	"gitlab.com/accumulatenetwork/core/schema/pkg/widget"
)

var binEncPool = binary.NewEncoderPool()
var binDecPool = binary.NewDecoderPool()
var bufferPool = binary.NewBufferPool()

func wValue[V any]() widget.Widget[**Value[V]] {
	return valueWidget[V]{}
}

func (v *Value[V]) Get() (V, error) {
	if v == nil || (!v.dataOk && !v.valueOk) {
		panic("invalid value")
	}
	if v.valueOk {
		return v.value, nil
	}

	dec := binDecPool.Get(bytes.NewReader(v.data))
	defer binDecPool.Put(dec)
	err := dec.Decode(&v.value)
	v.valueOk = err == nil
	return v.value, err
}

type valueWidget[V any] struct{}

func (valueWidget[V]) IsNil(v **Value[V]) bool { return *v == nil }
func (valueWidget[V]) Empty(v **Value[V]) bool { return *v == nil }

func (valueWidget[V]) CopyTo(dst, src **Value[V]) {
	if *src == nil {
		*dst = nil
		return
	}

	if *dst == nil {
		*dst = new(Value[V])
	}

	**dst = **src
	if v, ok := any((*src).value).(interface{ Copy() V }); ok {
		(*dst).value = v.Copy()
	}
}

func (w valueWidget[V]) Equal(a, b **Value[V]) bool {
	v, err := (*a).Get()
	if err != nil {
		return false
	}
	u, err := (*b).Get()
	if err != nil {
		return false
	}
	if v, ok := any(v).(interface{ Equal(interface{}) bool }); ok {
		return v.Equal(u)
	}
	return reflect.DeepEqual(v, u)
}

func (valueWidget[V]) MarshalJSON(e *json.Encoder, v **Value[V]) error {
	u, err := (*v).Get()
	if err != nil {
		return err
	}
	return e.Encode(u)
}

func (valueWidget[V]) UnmarshalJSON(d *json.Decoder, v **Value[V]) error {
	var z V
	err := d.Decode(&z)
	if err != nil {
		return err
	}

	*v = &Value[V]{value: z, valueOk: true}
	return nil
}

func (valueWidget[V]) MarshalBinary(e *binary.Encoder, v **Value[V]) error {
	u := *v
	if u.dataOk {
		return e.EncodeBytes(u.data)
	}
	if !u.valueOk {
		panic("invalid value")
	}

	buf := bufferPool.Get()
	defer bufferPool.Put(buf)

	enc := binEncPool.Get(buf)
	defer binEncPool.Put(enc)
	enc.Reset(buf, binary.WithBufferPool(bufferPool))

	err := enc.Encode(u.value)
	if err != nil {
		return err
	}

	b := buf.Bytes()
	u.data = make([]byte, len(b))
	copy(u.data, b)
	u.dataOk = true
	return e.EncodeBytes(u.data)
}

func (valueWidget[V]) UnmarshalBinary(d *binary.Decoder, v **Value[V]) error {
	b, err := d.DecodeBytes()
	if err != nil {
		return err
	}

	*v = &Value[V]{data: b, dataOk: true}
	return nil
}

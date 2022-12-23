// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package model

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

const rootStateSize = 1 + 2 + 2 + 32

func (r *RootState) MarshalBinary() ([]byte, error) {
	var data []byte
	data = append(data, byte(r.MaxHeight))
	data = append(data, byte(r.Power>>8), byte(r.Power))
	data = append(data, byte(r.Mask>>8), byte(r.Mask))
	data = append(data, r.RootHash[:]...)
	return data, nil
}

func (r *RootState) UnmarshalBinary(data []byte) error {
	if len(data) != rootStateSize {
		return encoding.ErrNotEnoughData
	}
	r.MaxHeight = uint64(data[0])
	r.Power = uint64(data[1])<<8 + uint64(data[2])
	r.Mask = uint64(data[3])<<8 + uint64(data[4])
	r.RootHash = *(*[32]byte)(data[5:])
	return nil
}

func (r *RootState) UnmarshalBinaryFrom(rd io.Reader) error {
	var buf [rootStateSize]byte
	_, err := io.ReadFull(rd, buf[:])
	if err != nil {
		return err
	}
	return r.UnmarshalBinary(buf[:])
}

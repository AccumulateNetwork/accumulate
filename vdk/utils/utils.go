// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package utils

import (
	"encoding/binary"
	"errors"
)

var ErrNotEnoughData = errors.New("not enough data")
var ErrOverflow = errors.New("overflow")

var _ = UvarintUnmarshalBinary

func UvarintUnmarshalBinary(b []byte) (uint64, error) {
	v, n := binary.Uvarint(b)
	if n == 0 {
		return 0, ErrNotEnoughData
	}
	if n < 0 {
		return 0, ErrOverflow
	}
	return v, nil
}

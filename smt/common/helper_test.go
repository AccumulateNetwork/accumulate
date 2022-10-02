// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package common

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSliceBytes(t *testing.T) {
	for i := 0; i < 1024; i++ {
		var bytetest []byte
		for j := 0; j < i; j++ {
			bytetest = append(bytetest, byte(rand.Int()))
		}
		counted := SliceBytes(bytetest)
		slice, left := BytesSlice(counted)

		if len(left) > 0 || !bytes.Equal(bytetest, slice) {
			t.Errorf(" %d %x %x", len(left), bytetest, slice)
		}
	}

	var tests [][]byte
	var inputs []byte
	for i := 0; i < 1024; i++ {
		var bytetest []byte
		for j := 0; j < i; j++ {
			bytetest = append(bytetest, byte(rand.Int()))
		}
		tests = append(tests, bytetest)
		counted := SliceBytes(bytetest)
		inputs = append(inputs, counted...)
	}

	for i, v := range tests {
		var slice []byte
		slice, inputs = BytesSlice(inputs)

		if !bytes.Equal(v, slice) {
			t.Errorf(" %d %x %x", i, v, slice)
		}
	}

	if len(inputs) != 0 {
		t.Error("should consume all data")
	}
}

func TestFormatTimeLapse(t *testing.T) {
	var d = time.Duration(time.Hour*3 + time.Minute*4 + time.Second*5)
	str := FormatTimeLapse(d)
	if fmt.Sprint(str) != "    03:04:05 h:m:s  " {
		t.Error("failed to print time as desired")
	}
}

func TestHexToBytes(t *testing.T) {
	test := "abcdef1234567890"
	bytes := hexToBytes(test)
	bs := fmt.Sprintf("%x", bytes)
	if bs != test {
		t.Error("hex to string didn't work")
	}

	testHex := func(hex string) (gotError bool) {
		defer func() {
			gotError = recover() != nil
		}()
		_ = hexToBytes(hex)
		return false
	}

	if !testHex("abcdefg") {
		t.Error("no error on odd length hex string")
	}
	if testHex("abcdef") {
		t.Error("error on an even length hex string")
	}
	if !testHex("abcdefhi") {
		t.Error("didn't error on invalid characters")
	}
	if testHex("abcdef") {
		t.Error("error on valid characters")
	}
}

func TestInt64FixedBytes(t *testing.T) {
	value := 0x0123456789987654
	iBytes := Uint64FixedBytes(uint64(value))
	require.Truef(t, len(iBytes) == 8, "fail len = %d", len(iBytes))
	v, data := BytesFixedUint64(iBytes)
	require.Truef(t, int(v) == value, "fail %x %x %x", iBytes, v, value)
	require.True(t, len(data) == 0, "fail")
}

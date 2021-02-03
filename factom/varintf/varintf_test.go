// MIT License
//
// Copyright 2018 Canonical Ledgers, LLC
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
// IN THE SOFTWARE.

package varintf

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecode(t *testing.T) {
	assert := assert.New(t)
	for x := uint64(1); x > 0; x <<= 1 {
		buf := Encode(x)
		d, l := Decode(buf)
		assert.Equalf(x, d, "x, x == %v", x)
		assert.Equalf(len(buf), l, "l, x == %v", x)
	}
}

var testFactomSpecExamples = []struct {
	X   uint64
	Buf []byte
}{{
	X:   0,
	Buf: []byte{0},
}, {
	X:   3,
	Buf: []byte{3},
}, {
	X:   127,
	Buf: []byte{127},
}, {
	X: 128,
	// 10000001 00000000
	Buf: []byte{0x81, 0},
}, {
	X: 130,
	// 10000001 00000010
	Buf: []byte{0x81, 2},
}, {
	X: (1 << 16) - 1, // 2^16 - 1
	// 10000011 11111111 01111111
	Buf: []byte{0x83, 0xff, 0x7f},
}, {
	X: 1 << 16, // 2^16
	// 10000100 10000000 00000000
	Buf: []byte{0x84, 0x80, 0},
}, {
	X: (1 << 32) - 1, // 2^32 - 1
	// 10001111 11111111 11111111 11111111 01111111
	Buf: []byte{0x8f, 0xff, 0xff, 0xff, 0x7f},
}, {
	X: 1 << 32, // 2^32
	// 10010000 10000000 10000000 10000000 00000000
	Buf: []byte{0x90, 0x80, 0x80, 0x80, 0x00},
}, {
	X: (1 << 63) - 1, // 2^63 - 1
	// 11111111 11111111 11111111 11111111 11111111 11111111 11111111 11111111 01111111
	Buf: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
}, {
	X: (1 << 64) - 1, // 2^64 - 1
	// 10000001 11111111 11111111 11111111 11111111 11111111 11111111 11111111 11111111 01111111
	Buf: []byte{0x81, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
}}

func TestFactomSpecExamples(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	for _, test := range testFactomSpecExamples {
		buf := Encode(test.X)
		require.Equalf(test.Buf, buf, "buf, x == %v", test.X)
		x, l := Decode(test.Buf)
		assert.Equalf(test.X, x, "x, x == %v", test.X)
		assert.Equalf(len(buf), l, "l, x == %v", test.X)
	}
}

// TestVarInt_Panic ensures the varint decoding will never panic from invalid data
func TestVarInt_Panic(t *testing.T) {
	assert := assert.New(t)
	// a quick helper func to test these things
	testDecode := func(expA uint64, expR int, data []byte, msg string) {
		a, r := Decode([]byte{179, 144})
		assert.Equal(expA, a, msg)
		assert.Equal(expR, r, msg)
	}
	t.Run("known invalid slices", func(t *testing.T) {
		testDecode(0, -1, []byte{179, 144}, "not enough bytes")
	})

	// The easiest way to search for panics is with random data
	t.Run("test error handling (no panic)", func(t *testing.T) {
		for i := 0; i < 1000; i++ {
			d := make([]byte, rand.Intn(20))
			rand.Read(d)
			if len(d) == 0 {
				continue // uninteresting
			}
			// We don't know the expected return, but it should not panic
			Decode(d)
		}
	})

}

func BenchmarkDecode(b *testing.B) {
	var buf []byte
	for i := 0; i < b.N; i++ {
		buf = Encode(uint64((1 << uint(i%64)) - i))
	}
	_ = buf
}
func BenchmarkEncodeDecode(b *testing.B) {
	var buf []byte
	var x uint64
	var l int
	for i := 0; i < b.N; i++ {
		buf = Encode(uint64((1 << uint(i%64)) - i))
		x, l = Decode(buf)
	}
	_ = buf
	_ = x
	_ = l

}

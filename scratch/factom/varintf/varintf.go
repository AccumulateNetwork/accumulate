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

// Package varintf implements Factom's varInt_F specification.
//
// The varInt_F specifications uses the top bit (0x80) in each byte as the
// continuation bit. If this bit is set, continue to read the next byte. If
// this bit is not set, then this is the last byte. The remaining 7 bits are
// the actual data of the number. The bytes are ordered big endian, unlike the
// varInt used by protobuf or provided by package encoding/binary.
//
// https://github.com/FactomProject/FactomDocs/blob/master/factomDataStructureDetails.md#variable-integers-varint_f
package varintf

import (
	"math/bits"
)

const continuationBitMask = 0x80

// BufLen returns the number of bytes required to encode x.
//
// This must be used when passing buffers to Put.
//
//      Put(buf[:BufLen(x)], x)
func BufLen(x uint64) int {
	// bitlen is the minimum number of bits required to represent x.
	bitlen := bits.Len64(x)

	// buflen is the number of bytes required to encode x to varInt_F. Each
	// byte can store 7 bits of x plus a continuation bit.
	buflen := bitlen / 7

	// At least one byte is required to represent 0 (bitlen == 0) or to
	// account for the remainder bits after division by 7.
	if bitlen == 0 || bitlen%7 > 0 {
		buflen++
	}
	return buflen
}

// Put the encoding of x into buf, which must be exactly equal to BufLen(x).
// For example,
//
//      Put(buf[:BufLen(x)], x)
//
// If len(buf) is not exactly equal to BufLen(x), garbage data will be written
// into buf.
func Put(buf []byte, x uint64) {
	for i := range buf {
		buf[i] = continuationBitMask | uint8(x>>uint((len(buf)-i-1)*7))
	}
	// Unset continuation bit in last byte.
	buf[len(buf)-1] &^= continuationBitMask
}

// Encode x as into a new []byte with length BufLen(x).
//
// Use Put to control the allocation of the slice.
func Encode(x uint64) []byte {
	buf := make([]byte, BufLen(x))
	Put(buf, x)
	return buf
}

// Decode buf into uint64 and return the number of bytes used.
//
// If buf encodes a number larger than 64 bits, 0 and -1 is returned.
func Decode(buf []byte) (uint64, int) {
	var buflen int
	var complete bool
	for _, b := range buf {
		buflen++
		if b&continuationBitMask == 0 {
			complete = true
			break
		}

		// We cannot decode more than 10 bytes.
		if buflen >= 10 {
			return 0, -1
		}
	}

	// If the 10th byte is used, the most significant byte may not be
	// greater than 1, since 63 bits have already been accounted for in the
	// least significant 9 bytes.
	if !complete || (buflen == 10 && buf[0] > 0x01|continuationBitMask) {
		return 0, -1
	}

	var x uint64
	for i, b := range buf[:buflen] {
		x |= uint64(b&^continuationBitMask) << uint((buflen-i-1)*7)
	}
	return x, buflen
}

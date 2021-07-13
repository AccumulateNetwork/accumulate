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

package jsonlen

// Int64 returns the encoded length of d as a JSON Number.
// Int64(0) == 1, Int64(-50) == 3, Int64(7654321) = 7
func Int64(d int64) int {
	sign := 0
	if d < 0 {
		sign++
		d *= -1
	}
	return sign + Uint64(uint64(d))
}

// Uint64 returns the encoded length of d as a JSON Number.
// Uint64(0) == 1, Int64(50) == 2, Int64(7654321) = 7
func Uint64(d uint64) int {
	l := 1
	for pow := uint64(10); d/pow > 0; pow *= 10 {
		l++
	}
	return l
}

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

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var int64Tests = []struct {
	Name string
	D    int64
	Len  int
}{{
	Name: "zero",
	Len:  1,
}, {
	Name: "two digits",
	D:    10,
	Len:  2,
}, {
	Name: "two digits",
	D:    99,
	Len:  2,
}, {
	Name: "three digits",
	D:    100,
	Len:  3,
}, {
	Name: "three digits",
	D:    999,
	Len:  3,
}, {
	Name: "four digits",
	D:    1000,
	Len:  4,
}, {
	Name: "four digits",
	D:    9999,
	Len:  4,
}, {
	Name: "ten digits",
	D:    1000000000,
	Len:  10,
}, {
	Name: "ten digits",
	D:    9999999999,
	Len:  10,
}}

func TestInt64(t *testing.T) {
	for _, test := range int64Tests {
		t.Run(test.Name, func(t *testing.T) {
			assert := assert.New(t)
			assert.Equal(test.Len, Int64(test.D))
			if test.D != 0 {
				assert.Equal(test.Len+1, Int64(-test.D), "negative")
			}
		})
	}

}

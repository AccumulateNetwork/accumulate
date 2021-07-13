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

package factom

import (
	"encoding"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	JSONBytesInvalidTypes     = []string{`{}`, `5.5`, `["hello"]`}
	JSONBytes32InvalidLengths = []string{
		`"00"`,
		`"000000000000000000000000000000000000000000000000000000000000000000"`}
	JSONBytesInvalidSymbol = `"x000000000000000000000000000000000000000000000000000000000000000"`
	JSONBytes32Valid       = `"da56930e8693fb7c0a13aac4d01cf26184d760f2fd92d2f0a62aa630b1a25fa7"`
)

type unmarshalJSONTest struct {
	Name string
	Data string
	Err  string
	Un   interface {
		encoding.TextUnmarshaler
		encoding.TextMarshaler
	}
	Exp interface {
		encoding.TextUnmarshaler
		encoding.TextMarshaler
	}
}

var unmarshalJSONtests = []unmarshalJSONTest{{
	Name: "Bytes32/valid",
	Data: `"DA56930e8693fb7c0a13aac4d01cf26184d760f2fd92d2f0a62aa630b1a25fa7"`,
	Un:   new(Bytes32),
	Exp: &Bytes32{0xDA, 0x56, 0x93, 0x0e, 0x86, 0x93, 0xfb, 0x7c, 0x0a, 0x13,
		0xaa, 0xc4, 0xd0, 0x1c, 0xf2, 0x61, 0x84, 0xd7, 0x60, 0xf2, 0xfd,
		0x92, 0xd2, 0xf0, 0xa6, 0x2a, 0xa6, 0x30, 0xb1, 0xa2, 0x5f, 0xa7},
}, {
	Name: "Bytes/valid",
	Data: `"DA56930e8693fb7c0a13aac4d01cf26184d760f2fd92d2f0a62aa630b1a25fa7"`,
	Un:   new(Bytes),
	Exp: &Bytes{0xDA, 0x56, 0x93, 0x0e, 0x86, 0x93, 0xfb, 0x7c, 0x0a, 0x13,
		0xaa, 0xc4, 0xd0, 0x1c, 0xf2, 0x61, 0x84, 0xd7, 0x60, 0xf2, 0xfd,
		0x92, 0xd2, 0xf0, 0xa6, 0x2a, 0xa6, 0x30, 0xb1, 0xa2, 0x5f, 0xa7},
}, {
	Name: "Bytes32/valid",
	Data: `"0000000000000000000000000000000000000000000000000000000000000000"`,
	Un:   new(Bytes32),
	Exp:  &Bytes32{},
}, {
	Name: "Bytes/valid",
	Data: `"0000000000000000000000000000000000000000000000000000000000000000"`,
	Un:   new(Bytes),
	Exp:  func() *Bytes { b := make(Bytes, 32); return &b }(),
}, {
	Name: "invalid symbol",
	Data: `"DA56930e8693fb7c0a13aac4d01cf26184d760f2fd92d2f0a62aa630b1zxcva7"`,
	Err:  "encoding/hex: invalid byte: U+007A 'z'",
}, {
	Name: "invalid type",
	Data: `{}`,
	Err:  "json: cannot unmarshal object into Go value of type *factom.Bytes",
	Un:   new(Bytes),
}, {
	Name: "invalid type",
	Data: `{}`,
	Err:  "json: cannot unmarshal object into Go value of type *factom.Bytes32",
	Un:   new(Bytes32),
}, {
	Name: "invalid type",
	Data: `5.5`,
	Err:  "json: cannot unmarshal number into Go value of type *factom.Bytes",
	Un:   new(Bytes),
}, {
	Name: "invalid type",
	Data: `5.5`,
	Err:  "json: cannot unmarshal number into Go value of type *factom.Bytes32",
	Un:   new(Bytes32),
}, {
	Name: "invalid type",
	Data: `["asdf"]`,
	Err:  "json: cannot unmarshal array into Go value of type *factom.Bytes",
	Un:   new(Bytes),
}, {
	Name: "invalid type",
	Data: `["asdf"]`,
	Err:  "json: cannot unmarshal array into Go value of type *factom.Bytes32",
	Un:   new(Bytes32),
}, {
	Name: "too long",
	Data: `"DA56930e8693fb7c0a13aac4d01cf26184d760f2fd92d2f0a62aa630b1a25fa71234"`,
	Err:  "invalid length",
	Un:   new(Bytes32),
}, {
	Name: "too short",
	Data: `"DA56930e8693fb7c0a13aac4d01cf26184d760f2fd92d2f0a62aa630b1a25fa71234"`,
	Err:  "invalid length",
	Un:   new(Bytes32),
}}

func testUnmarshalJSON(t *testing.T, test unmarshalJSONTest) {
	assert := assert.New(t)
	err := json.Unmarshal([]byte(test.Data), test.Un)
	if len(test.Err) > 0 {
		assert.EqualError(err, test.Err)
		return
	}
	assert.NoError(err)
	assert.Equal(test.Exp, test.Un)
}

func TestBytes(t *testing.T) {
	for _, test := range unmarshalJSONtests {
		if test.Un != nil {
			t.Run("UnmarshalJSON/"+test.Name, func(t *testing.T) {
				testUnmarshalJSON(t, test)
			})
			if test.Exp != nil {
				t.Run("MarshalJSON/"+test.Name, func(t *testing.T) {
					data, err := json.Marshal(test.Un)
					assert := assert.New(t)
					assert.NoError(err)
					assert.Equal(strings.ToLower(test.Data),
						string(data))
				})
			}
			continue
		}
		test.Un = new(Bytes32)
		t.Run("UnmarshalJSON/Bytes32/"+test.Name, func(t *testing.T) {
			testUnmarshalJSON(t, test)
		})
		test.Un = new(Bytes)
		t.Run("UnmarshalJSON/Bytes/"+test.Name, func(t *testing.T) {
			testUnmarshalJSON(t, test)
		})
	}

	t.Run("IsZero()", func(t *testing.T) {
		assert.True(t, Bytes32{}.IsZero())
		assert.False(t, Bytes32{0: 1}.IsZero())
	})
}

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

package fat

import (
	"testing"

	"github.com/AccumulateNetwork/accumulated/factom"
	"github.com/stretchr/testify/assert"
)

var validIdentityChainID = factom.NewBytes32(
	"88888807e4f3bbb9a2b229645ab6d2f184224190f83e78761674c2362aca4425")

func validNameIDs() []factom.Bytes {
	return []factom.Bytes{
		factom.Bytes("token"),
		factom.Bytes("valid"),
		factom.Bytes("issuer"),
		validIdentityChainID[:],
	}
}

func TestNameIDs(t *testing.T) {
	nameIDs := NameIDs("valid", &validIdentityChainID)
	assert.ElementsMatch(t, validNameIDs(), nameIDs)
}
func TestParseTokenIssuer(t *testing.T) {
	token, identity := ParseTokenIssuer(validNameIDs())
	assert.Equal(t, "valid", token)
	assert.Equal(t, validIdentityChainID, identity)
}

func TestChainID(t *testing.T) {
	expected := factom.NewBytes32(
		"b54c4310530dc4dd361101644fa55cb10aec561e7874a7b786ea3b66f2c6fdfb")
	computed := ComputeChainID("test", &validIdentityChainID)
	assert.Equal(t, expected, computed)
}

func invalidNameIDs(i int) []factom.Bytes {
	n := validNameIDs()
	n[i] = factom.Bytes{}
	return n
}

var validNameIDsTests = []struct {
	Name    string
	NameIDs []factom.Bytes
	Valid   bool
}{{
	Name:    "valid",
	Valid:   true,
	NameIDs: validNameIDs(),
}, {
	Name:    "invalid length (short)",
	NameIDs: validNameIDs()[0:3],
}, {
	Name:    "invalid length (long)",
	NameIDs: append(validNameIDs()[:], factom.Bytes{}),
}, {
	Name:    "invalid",
	NameIDs: invalidNameIDs(0),
}, {
	Name:    "invalid ExtID",
	NameIDs: invalidNameIDs(1),
}, {
	Name:    "invalid ExtID",
	NameIDs: invalidNameIDs(2),
}, {
	Name:    "invalid ExtID",
	NameIDs: invalidNameIDs(3),
}}

func TestValidNameIDs(t *testing.T) {
	for _, test := range validNameIDsTests {
		t.Run(test.Name, func(t *testing.T) {
			assert := assert.New(t)
			valid := ValidNameIDs(test.NameIDs)
			if test.Valid {
				assert.True(valid)
			} else {
				assert.False(valid)
			}
		})
	}
}

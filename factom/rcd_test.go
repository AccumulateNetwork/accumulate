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

package factom_test

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	. "github.com/Factom-Asset-Tokens/factom"
	"github.com/stretchr/testify/assert"
)

var rcdUnmarshalBinaryTests = []struct {
	Name  string
	Data  []byte
	Error string
	RCDType
	Address Bytes32
}{
	{
		Name: "valid",
		Data: NewBytes(
			"010fd93026041de6387d2dcef0917c06288e690fa7652c20f044746e787b06b2bd"),
		RCDType: 1,
		Address: NewBytes32("304d80538e27505d44d5ff0ada6a9d420d93a9994da75f0763c12c827b616668"),
	},
	{
		Name: "invalid (too short)",
		Data: NewBytes(
			""),
		Error: "invalid RCD size",
	},
	{
		Name: "invalid (too short)",
		Data: NewBytes(
			"010fd93026041de6387d2dcef0917c06288e690fa7652c20f044746e787b06b2"),
		Error: "invalid RCDType01 size",
	},
	{
		Name: "invalid (unsupported rcd type)",
		Data: NewBytes(
			"000fd93026041de6387d2dcef0917c06288e690fa7652c20f044746e787b06b2bd"),
		Error: "unknown RCDType00",
	},
	{
		Name: "invalid (unsupported rcd type)",
		Data: NewBytes(
			"020fd93026041de6387d2dcef0917c06288e690fa7652c20f044746e787b06b2bd"),
		Error: "unknown RCDType02",
	},
}

func TestDecodeRCD(t *testing.T) {
	for _, test := range rcdUnmarshalBinaryTests {
		t.Run("UnmarshalBinary/"+test.Name, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)
			var rcd RCD
			err := rcd.UnmarshalBinary(test.Data)
			r := len(rcd)
			if len(test.Error) == 0 {
				require.NoError(err)
				require.Equal(r, len(test.Data))
				require.NotNil(rcd)

				assert.Equal(rcd.Type(), test.RCDType)
				addr := rcd.Hash()
				assert.Equal(addr[:], test.Address[:])
			} else {
				require.EqualError(err, test.Error)
			}
		})
	}

	// To ensure there is not panics and all errors are caught
	t.Run("UnmarshalBinary/RCD", func(t *testing.T) {
		for i := 0; i < 1000; i++ {
			d := make([]byte, rand.Intn(500))
			rand.Read(d)

			var rcd RCD
			err := rcd.UnmarshalBinary(d)
			r := len(rcd)
			if err == nil {
				// The RCD 1 type is pretty easy to be randomly valid.
				// So if it is type one and we read the right number of bytes,
				// there is no expected error.
				if r != 33 && rcd.Type() != 1 && d[0] == 0x01 {
					t.Errorf("expected an error")
				}
			}
		}
	})
}

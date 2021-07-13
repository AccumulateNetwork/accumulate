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

var fblockUnmarshalBinaryTests = []struct {
	Name        string
	Data        []byte
	Error       string
	KeyMr       Bytes32
	BodyMR      Bytes32
	LedgerKeyMR Bytes32
	Expansion   Bytes
}{
	{
		Name: "valid (block 100,000 on mainnet)",
		Data: NewBytes(
			"000000000000000000000000000000000000000000000000000000000000000f4d3c6399395f861bfb1ed3d4c44045f92ba33e4190a9802332fd161682881559e83db6d3b5341117ed5d30c169ca46a0b71520b637730f6d427beffcdf544c865173314fc27c7df0b010e69ff1b33a11b02b070106bf0584e8b6d0e9160245450000000000001194000186a000000000050000041502015da7414a5700000002015da7410114010100acda899570f75e5e909cc93bf80a7c81251a58b0a15b77be8b38451d99a931d738ccde18caacda85f00088cbf33350d13de4b71779adb908f5ddd92cd62033345518a33399f69e257a0701c2020ce54a88d09d72a225d25d6d23f43380a71d5b0192ec728c8c30d92b997909097ab4cc72eb540f069f989d3837e24dcfcaf4417c8b58da594e17cee8445f681822dd3a374ac00caf60539a6ab06e53eeb65f1bad7372923de4689b99770f0002015da7438e68020100acda85f00088cbf33350d13de4b71779adb908f5ddd92cd62033345518a33399f69e257a0783c904330fd717584445ac866dc2facd8b856e63bdb8b15b5ed46c0b053b2c6c5c5c3facda85f000330fd717584445ac866dc2facd8b856e63bdb8b15b5ed46c0b053b2c6c5c5c3f01ebf6c89d430bd27a9439553bff4122feb2a7e89cce9de9e880f4e5d12b32f1c69ffc856be77a8c10b1fed5b5a0ca18d9a7eafae1e9c363954477ad5e4f1fb489a3c4355dbd540a6ce9093fe6123ac6211355831e0a4672e3125d1c9edd279208012c94f2bbe49899679c54482eba49bf1d024476845e478f9cce3238f612edd761c068a515c81b927e414d3f955ce909ae8457a6c859dddc572caafbc3528aa9dc6c9141b52d61c59c7471602f8c14ff34450c07dd3e3ab67cfbbd5cb9af40c00c000000000002015da7475236010200b1a793895bf75e5e909cc93bf80a7c81251a58b0a15b77be8b38451d99a931d738ccde18ca8ae4cdc223894a4a7b8c666c6e280e5bfd258ff531bbbf3afc251826a399cc8b5f05aa7706a6c2bfc2006f94af1f895ce348cb6683d0fffb1144451c394885ab18d64a7470f85f39fcfb01c2020ce54a88d09d72a225d25d6d23f43380a71d5b0192ec728c8c30d92b99798f8a2bcddf5a1bced799fcec8f2550859e1cad4e1aeda70be7a57403d6c50241f2bea92904b049d0decdf0e1c28b0fe20ec17a6ffef1eb83903b62ce6a7c68060002015da748c2d40201008ae4cdc223894a4a7b8c666c6e280e5bfd258ff531bbbf3afc251826a399cc8b5f05aa770683c904330fd717584445ac866dc2facd8b856e63bdb8b15b5ed46c0b053b2c6c5c5c3f8ae4cdc223330fd717584445ac866dc2facd8b856e63bdb8b15b5ed46c0b053b2c6c5c5c3f016b12ae1a61a9675ea21d1ab6dbcf640a2a5cccd9f4c0c40b00143e02b8975b04caf15d9bfa27c9141487153d411ad12e1504a9a0b0ecdabb154ea59be0461295e2a5b4bd957daa34ba9a2bf00635eb7108d9e655bf6204e8deefc432161ce405012c94f2bbe49899679c54482eba49bf1d024476845e478f9cce3238f612edd76108622d4a69ef8acc6a5fec6706ab32acbdc41a45dcd555a3a99ac3d93ba3dfd86908221bd961d3be248dc7a0ae942b93ae856545594096450a99fbd05f4f980b000000"),
		KeyMr:       NewBytes32("199d98365896655907f513b2a433afb0129179035e7c0554aa40eb34ef238b12"),
		BodyMR:      NewBytes32("4d3c6399395f861bfb1ed3d4c44045f92ba33e4190a9802332fd161682881559"),
		LedgerKeyMR: NewBytes32("90d5b525a1300d77f23faf69b5fef53ce3f739805a0045c68d6ccf57b5685e84"),
	},
	{
		// Expansion bytes are []byte("Random expansion bytes")
		Name: "valid (block 100,000 on mainnet with expansion bytes)",
		Data: NewBytes(
			"000000000000000000000000000000000000000000000000000000000000000f4d3c6399395f861bfb1ed3d4c44045f92ba33e4190a9802332fd161682881559e83db6d3b5341117ed5d30c169ca46a0b71520b637730f6d427beffcdf544c865173314fc27c7df0b010e69ff1b33a11b02b070106bf0584e8b6d0e9160245450000000000001194000186a01652616e646f6d20657870616e73696f6e206279746573000000050000041502015da7414a5700000002015da7410114010100acda899570f75e5e909cc93bf80a7c81251a58b0a15b77be8b38451d99a931d738ccde18caacda85f00088cbf33350d13de4b71779adb908f5ddd92cd62033345518a33399f69e257a0701c2020ce54a88d09d72a225d25d6d23f43380a71d5b0192ec728c8c30d92b997909097ab4cc72eb540f069f989d3837e24dcfcaf4417c8b58da594e17cee8445f681822dd3a374ac00caf60539a6ab06e53eeb65f1bad7372923de4689b99770f0002015da7438e68020100acda85f00088cbf33350d13de4b71779adb908f5ddd92cd62033345518a33399f69e257a0783c904330fd717584445ac866dc2facd8b856e63bdb8b15b5ed46c0b053b2c6c5c5c3facda85f000330fd717584445ac866dc2facd8b856e63bdb8b15b5ed46c0b053b2c6c5c5c3f01ebf6c89d430bd27a9439553bff4122feb2a7e89cce9de9e880f4e5d12b32f1c69ffc856be77a8c10b1fed5b5a0ca18d9a7eafae1e9c363954477ad5e4f1fb489a3c4355dbd540a6ce9093fe6123ac6211355831e0a4672e3125d1c9edd279208012c94f2bbe49899679c54482eba49bf1d024476845e478f9cce3238f612edd761c068a515c81b927e414d3f955ce909ae8457a6c859dddc572caafbc3528aa9dc6c9141b52d61c59c7471602f8c14ff34450c07dd3e3ab67cfbbd5cb9af40c00c000000000002015da7475236010200b1a793895bf75e5e909cc93bf80a7c81251a58b0a15b77be8b38451d99a931d738ccde18ca8ae4cdc223894a4a7b8c666c6e280e5bfd258ff531bbbf3afc251826a399cc8b5f05aa7706a6c2bfc2006f94af1f895ce348cb6683d0fffb1144451c394885ab18d64a7470f85f39fcfb01c2020ce54a88d09d72a225d25d6d23f43380a71d5b0192ec728c8c30d92b99798f8a2bcddf5a1bced799fcec8f2550859e1cad4e1aeda70be7a57403d6c50241f2bea92904b049d0decdf0e1c28b0fe20ec17a6ffef1eb83903b62ce6a7c68060002015da748c2d40201008ae4cdc223894a4a7b8c666c6e280e5bfd258ff531bbbf3afc251826a399cc8b5f05aa770683c904330fd717584445ac866dc2facd8b856e63bdb8b15b5ed46c0b053b2c6c5c5c3f8ae4cdc223330fd717584445ac866dc2facd8b856e63bdb8b15b5ed46c0b053b2c6c5c5c3f016b12ae1a61a9675ea21d1ab6dbcf640a2a5cccd9f4c0c40b00143e02b8975b04caf15d9bfa27c9141487153d411ad12e1504a9a0b0ecdabb154ea59be0461295e2a5b4bd957daa34ba9a2bf00635eb7108d9e655bf6204e8deefc432161ce405012c94f2bbe49899679c54482eba49bf1d024476845e478f9cce3238f612edd76108622d4a69ef8acc6a5fec6706ab32acbdc41a45dcd555a3a99ac3d93ba3dfd86908221bd961d3be248dc7a0ae942b93ae856545594096450a99fbd05f4f980b000000"),
		KeyMr:       NewBytes32("cf7765b388f75c6dd98599d742a7c1c4da56bbe048ee99bc229741858c55552d"),
		BodyMR:      NewBytes32("4d3c6399395f861bfb1ed3d4c44045f92ba33e4190a9802332fd161682881559"),
		LedgerKeyMR: NewBytes32("4abb2a614e7cfb6bd8b9d7cf818731b3f36ae40b45ec35b52d729823c5d829d4"),
		Expansion:   []byte("Random expansion bytes"),
	},
	// TODO: Add invalid tests
	{
		Name: "invalid (no data)",
		Data: NewBytes(
			""),
		Error: "insufficient length",
	},
	{
		Name: "invalid (bad fct chain)",
		Data: append(NewBytes(
			"000000000000000000000000000000000000000000000000000000000000000a"), make([]byte, 300)...),
		Error: "invalid factoid chainid",
	},
}

func TestFBlock_UnmarshalBinary(t *testing.T) {
	for _, test := range fblockUnmarshalBinaryTests {
		t.Run("UnmarshalBinary/"+test.Name, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)
			f := FBlock{}
			err := f.UnmarshalBinary(test.Data)
			if len(test.Error) == 0 {
				require.NoError(err)
				require.NotNil(f.BodyMR)

				data, err := f.MarshalBinary()
				require.NoError(err)
				assert.Equal(test.Data, data)

				assert.Equal(test.BodyMR[:], f.BodyMR[:])

				assert.Equal(test.KeyMr[:], f.KeyMR[:])

				if len(test.Expansion) > 0 {
					assert.Equal(test.Expansion[:],
						f.Expansion[:])
				}
			} else {
				require.EqualError(err, test.Error)
			}
		})

		// Retrieving the values from the Get which "hits" factomd
		t.Run("Get/UnmarshalBinary/"+test.Name, func(t *testing.T) {
			if test.Error != "" {
				return // This fblock is invalid, so cannot be got
			}

			if test.KeyMr.IsZero() {
				return // Need a keymr to try the Get
			}

			// assert := assert.New(t)
			assert := assert.New(t)
			require := require.New(t)

			// Testing fetching the fblock by keymr
			f := FBlock{KeyMR: &test.KeyMr}
			cl := ClientWithFixedRPCResponse(struct {
				Data Bytes `json:"data"`
			}{test.Data})
			factomClient := NewClient()
			factomClient.Factomd.Client = *cl
			err := f.Get(nil, factomClient)
			require.NoError(err)

			assert.Equal(test.KeyMr, *f.KeyMR)
			assert.Equal(test.LedgerKeyMR, *f.LedgerKeyMR)
		})
	}

	// To ensure there is not panics and all errors are caught
	t.Run("UnmarshalBinary/Random", func(t *testing.T) {
		buf := make([]byte, 20000)
		fBlockChainID := FBlockChainID()
		for i := 0; i < 5000; i++ {
			d := buf[:rand.Intn(15000)+5000]
			rand.Read(d)

			if len(d) > 32 {
				copy(d[:32], fBlockChainID[:])
			}

			var fb FBlock
			err := fb.UnmarshalBinary(d)
			if err == nil {
				t.Errorf("expected an error")
			}
		}
	})
}

func TestFBlock_IsPopulated(t *testing.T) {
	fblock := FBlock{}
	if fblock.IsPopulated() {
		t.Error("Should be unpopulated")
	}
}

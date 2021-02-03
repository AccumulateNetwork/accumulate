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
	"time"

	"github.com/stretchr/testify/require"

	. "github.com/Factom-Asset-Tokens/factom"
	"github.com/stretchr/testify/assert"
)

var txMarshalBinaryTests = []struct {
	Name     string
	TxID     Bytes32
	FullHash Bytes32
	Transaction
}{{
	Name:     "valid",
	TxID:     NewBytes32("c7ea8854be1456ed8588b33d8d3cc2d90b911fcb01ec902ca3202e8bdbf269bf"),
	FullHash: NewBytes32("64251aa63e011f803c883acf2342d784b405afa59e24d9c5506c84f6c91bf18b"),
	Transaction: Transaction{
		ID:            nil,
		TimestampSalt: time.Unix(0, 1443537161594*1e6),
		// Keep the count info, hard to decode with your eyes...
		// InputCount:     1,
		// FCTOutputCount: 1,
		// ECOutputCount:  0,
		FCTInputs: []AddressAmount{
			{
				Amount:  1000000000,
				Address: NewBytes("ab87d1b89117aba0e6b131d1ee42f99c6e806b76ca68c0823fb65c80d694b125"),
			},
		},
		FCTOutputs: []AddressAmount{
			{
				Amount:  992000800,
				Address: NewBytes("648f374d3de5d1541642c167a34d0f7b7d92fd2dab4d32f313776fa5a2b73a98"),
			},
		},
		ECOutputs: nil,
		Signatures: []RCDSignature{
			{
				// RCD 0110560cc304eb0a3b0540bc387930d2a7b2373270cfbd8448bc68a867cefb9f74
				RCD:       RCD(NewBytes("0110560cc304eb0a3b0540bc387930d2a7b2373270cfbd8448bc68a867cefb9f74")),
				Signature: NewBytes("d68d5ce3bee5e69f113d643df1f6ba0dd476ada40633751537d5b840e2be811d4734ef9f679966fa86b1777c8a387986b4e21987174f9df808c2081be2c04a08"),
			},
		},
	},
}}

func TestTransactionMarshalBinary(t *testing.T) {
	for _, test := range txMarshalBinaryTests {
		t.Run(test.Name, func(t *testing.T) {
			require := require.New(t)
			data, err := test.Transaction.MarshalBinary()
			require.NoError(err)

			var tx Transaction
			require.NoError(tx.UnmarshalBinary(data), Bytes(data))
			require.Equal(test.Transaction, tx)
		})
	}
}

var txUnmarshalBinaryTests = []struct {
	Name     string
	Data     []byte
	Error    string
	Valid    bool
	TxID     Bytes32
	FullHash Bytes32
}{
	{
		Name: "valid (1 fct out)",
		Data: NewBytes(
			"020162606d234b01010092d097e400304d80538e27505d44d5ff0ada6a9d420d93a9994da75f0763c12c827b61666892d0969560b11e86b4894661091c16f511a2f1000099b54dcf73bc7bcacba6e3fe2f547c83010fd93026041de6387d2dcef0917c06288e690fa7652c20f044746e787b06b2bdf7f1ec53e7e5695667b071b4e3ae65c09c804e164e971ac4717c7a8d0b61053f831f605e0adbf98d19613b00797962beb899a8d9472e187b17444159e74b2f0e"),
		TxID: NewBytes32(
			"637d9e8a7464aa272192e499bb0cdc720288884bfd5546bab8dcb8892527fc7b"),
		FullHash: NewBytes32(
			"59b293f8ade00f7f51d42762b864e287ea31d195563e1970356fe8c1d49fce97"),
		Valid: true,
	},
	{
		Name: "valid (1 ec out)",
		Data: NewBytes(
			"02016d16b4aac301000183dd95b22031a14669cd993de61b28454222c75e33f3a3a51eddd8394a150b0068778e9ebc83dd80c2304fe2a4a9debe6bd58e4424c9e1d00dc9e71a23c8c6b9e9db96d478012181cc0801b60844047ee177e4e22bdd48819694fdec025f432f482e820e06d0023befe690e295e854af57c60e8610a28dd743f5427a868f48c10e41e1373c01537d91bea1f0703058e0dd8a5620b81746d6b435f0ddc35daeee496d5dd8f5134fed34490d"),
		TxID: NewBytes32(
			"ff61ee20adda5cbb4ab2eb583881e94b707746660900c6a55c11e3a1e5a7de69"),
		FullHash: NewBytes32(
			"d8aec7bf1361e2dfeaf7806ab8e00c1bb49a931932abe561fabb0833cddef4c3"),
		Valid: true,
	},
	{
		Name: "valid (4 fct in, 3 fct out)",
		Data: NewBytes(
			"020154e64948850403008efca5d4105706ce61e91f33eb5506e86a717285a4dbcedb6678e8ffc326a3b11d3aa36123c0dbe8826b53435eb1ee2692ce5a9cb356eba3bfa8dd93f6470551812259d57bd4c13d629292f8b44ed1f8785ae9a13126d566c6cbed3a592b16570d3a7928fe3f6a1782b1caeaee5492e4d7deae75c641be9c4b1ad1d937829c79c397d70c3c2f2745a2d66b9012cfea55bbbba15182dff0d6ff1854053af90420a3c455828e5e7ef5057ebeae2bcf6fb9db62a8de66b1a26e158986b1aaa5f17048d2e5a20571043b492abc1a9d40072c31ee86c0a408b6ef3fd5efc2a7462a328aa3a6dcdb7e8ef427e0978bd09f941a8d2586ce1c6401f2d7663914c6c350555e0bfe32ac010171230c7848588d5de3855fd4d526e302e9c8a309ab591335b5d1d3a071e583f363aee6fbc0ea9bf703644f8199612e2029fdd4f43cfc28d73811228d7b35d88773f59019440e4646b3e8d499cd7e2b1171693dc071543a1e330c22057038ab00012aca1b1cf03adf48039dcca868d93cfe30faa7c931703a3c397b96f846cfb373537273acde3366b8c0d675ce6fd0d3f9f3f0800aaf0afd5b750c482ab12ca52b86fadcb7aba08fb3430f4fab9acf90f2d60daddd9cd9da65143e247339584a0401dd6db3e73e140c28d1f003992ee084623de8b4e955e89c20df73d0d899ad86b0d9d0e5e988031c28879e028ef80301f42fdebb0993dc213a49bc117b24617a151ff200d1d2d907735b7ad90fde18f914e187abaa6c5f4add357113363a67d407012c2df5f3987d34a9ec9a814d6017122a1e78d4569785c7bc8277ed3f8c1e3957ad90bbbd9f2eb774f8e2713a37721a2f7a5e3ae510eacdeeff362e44ade6037a8d33745eced481d9e72527702d5cd887fe19295f0ee204807dc6ce76d1c6ba01"),
		TxID: NewBytes32(
			"fa323daccaa9959b27b5a2f4efa5938a3ba9c99e11ef7af6406035d2e06ce286"),
		FullHash: NewBytes32(
			"0bc0affe278911b1c59e3ffbe803842787d3d76c3ea42d68250a5b5de5e2b420"),
		Valid: true,
	},
	{
		Name: "valid (coinbase)",
		Data: NewBytes(
			"02015da7414a57000000"),
		TxID: NewBytes32(
			"406dbd6e0f09352f079d4a41b3d9fa57b91a0df131ad6198d68f829663ddaece"),
		FullHash: NewBytes32(
			"406dbd6e0f09352f079d4a41b3d9fa57b91a0df131ad6198d68f829663ddaece"),
		Valid: true,
	},
	// Invalid Sigs
	{
		Name: "valid but rcd does not match input",
		Data: NewBytes(
			"02a162606d234b01010092d097e400304d80538e27505d44d5ff0ada6a9d420d93a9994da75f0763c12c827b61666892d0969560b11e86b4894661091c16f511a2f1000099b54dcf73bc7bcacba6e3fe2f547c83010fd93026041de6387d2dcef0917c06288e690fa7652c20f044746e787b06b2bdf7f1ec53e7e5695667b071b4e3ae65c09c804e164e971ac4717c7a8d0b61053f831f605e0adbf98d19613b00797962beb899a8d9472e187b17444159e74b2f0e"),
		Valid: false,
	},
	// Invalid Marshals
	{
		Name: "invalid (too short)",
		Data: NewBytes(
			""),
		Error: "insufficient length",
	},
	{
		Name: "invalid (txid set is incorrect)",
		Data: NewBytes(
			"020162606d234b01010092d097e400304d80538e27505d44d5ff0ada6a9d420d93a9994da75f0763c12c827b61666892d0969560b11e86b4894661091c16f511a2f1000099b54dcf73bc7bcacba6e3fe2f547c83010fd93026041de6387d2dcef0917c06288e690fa7652c20f044746e787b06b2bdf7f1ec53e7e5695667b071b4e3ae65c09c804e164e971ac4717c7a8d0b61053f831f605e0adbf98d19613b00797962beb899a8d9472e187b17444159e74b2f0e"),
		TxID: NewBytes32(
			"aa7d9e8a7464aa272192e499bb0cdc720288884bfd5546bab8dcb8892527fc7b"),
		FullHash: NewBytes32(
			"59b293f8ade00f7f51d42762b864e287ea31d195563e1970356fe8c1d49fce97"),
		Error: "invalid TxID",
	},
}

func TestTransaction_UnmarshalBinary(t *testing.T) {
	for _, test := range txUnmarshalBinaryTests {
		t.Run("UnmarshalBinary/"+test.Name, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)
			var tx Transaction
			if !test.TxID.IsZero() {
				tx.ID = &test.TxID
			}

			err := tx.UnmarshalBinary(test.Data)
			if len(test.Error) == 0 {
				require.NoError(err)
				require.NotNil(tx.FCTInputs)
				if !test.TxID.IsZero() {
					assert.Equal(test.TxID[:], tx.ID[:])
				}
				assert.Equal(tx.MarshalBinaryLen(), len(test.Data))
			} else {
				require.EqualError(err, test.Error)
			}
		})
	}

	// To ensure there is not panics and all errors are caught
	t.Run("UnmarshalBinary/Transaction", func(t *testing.T) {
		for i := 0; i < 10000; i++ {
			d := make([]byte, rand.Intn(1000))
			rand.Read(d)

			var tx Transaction
			err := tx.UnmarshalBinary(d)
			if err == nil {
				t.Errorf("expected an error")
			}
		}
	})
}

func TestTransaction_IsPopulated(t *testing.T) {
	var tx Transaction
	if tx.IsPopulated() {
		t.Errorf("Should not be populated")
	}
}

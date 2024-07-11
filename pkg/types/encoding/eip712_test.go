// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding_test

import (
	"bytes"
	"encoding/hex"
	"os"
	"os/exec"
	"testing"

	eth "github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestEIP712Arrays(t *testing.T) {
	acctesting.SkipWithoutTool(t, "node")

	// These aren't supposed to be a real transactions, it's just an easy test
	// case for how our EIP-712 implementation handles arrays. Specifically
	// there's an array with two values
	cases := map[string]string{
		"SendTokens": `{
			"header": {
				"principal": "acc://adi.acme/ACME"
			},
			"body": {
				"type": "sendTokens",
				"to": [
					{"url": "acc://other.acme/ACME"},
					{"amount": "10000000000"}
				]
			}
		}`,

		"UpdateKeyPage": `{
			"header": {
				"principal": "acc://adi.acme/ACME"
			},
			"body": {
				"type": "updateKeyPage",
				"operations": [
					{"type": "add", "entry": { "keyHash": "c0ffee" }},
					{"type": "setThreshold", "threshold": 1}
				]
			}
		}`,
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			txn := &protocol.Transaction{}
			err := txn.UnmarshalJSON([]byte(src))
			require.NoError(t, err)

			priv := acctesting.NewSECP256K1(t.Name())
			sig := &protocol.Eip712TypedDataSignature{
				PublicKey:     eth.FromECDSAPub(&priv.PublicKey),
				Signer:        url.MustParse("acc://adi.acme/book/1"),
				SignerVersion: 1,
				Timestamp:     1720564975623,
			}
			txn.Header.Initiator = [32]byte(sig.Metadata().Hash())

			b, err := protocol.MarshalEip712(txn, sig)
			require.NoError(t, err)

			cmd := exec.Command("../../../test/cmd/eth_signTypedData/execute.sh", hex.EncodeToString(priv.Serialize()), string(b))
			cmd.Stderr = os.Stderr
			out, err := cmd.Output()
			require.NoError(t, err)

			out = bytes.TrimSpace(out)
			out = bytes.TrimPrefix(out, []byte("0x"))
			sig.Signature = make([]byte, len(out)/2)
			_, err = hex.Decode(sig.Signature, out)
			require.NoError(t, err)
			require.True(t, sig.Verify(nil, txn))
		})
	}
}

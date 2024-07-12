// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package encoding_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"testing"

	eth "github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestEIP712Arrays(t *testing.T) {
	acctesting.SkipWithoutTool(t, "node")

	// These aren't supposed to be a real transactions, it's just an easy test
	// case for how our EIP-712 implementation handles arrays. Specifically
	// there's an array with two values
	cases := []*protocol.Transaction{
		must(build.Transaction().
			For("adi.acme", "book", "1").
			UpdateKeyPage().
			Add().Entry().Hash([32]byte{1, 2, 3}).FinishEntry().FinishOperation().
			Add().Entry().Owner("foo.bar").FinishEntry().FinishOperation().
			Done()),
		must(build.Transaction().
			For("adi.acme", "book", "1").
			UpdateKeyPage().
			Add().Entry().Hash([32]byte{1, 2, 3}).FinishEntry().FinishOperation().
			SetThreshold(2).
			Done()),
	}

	for _, txn := range cases {
		t.Run(hex.EncodeToString(txn.GetHash()), func(t *testing.T) {
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
			fmt.Printf("%s\n", b)

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

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}

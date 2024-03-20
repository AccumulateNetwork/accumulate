// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestSignatureMemo(t *testing.T) {
	// Tests AIP-006
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.Genesis(GenesisTime),
	)

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

	// Submit
	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice, "book", "1").
			BurnCredits(1).
			SignWith(alice, "book", "1").
			Version(1).Timestamp(1).
			Memo("foo").
			Metadata("bar").
			PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st[0].TxID).Succeeds())

	// Verify the signature
	sig := sim.QueryMessage(st[1].TxID, nil).Message.(*messaging.SignatureMessage).Signature.(*protocol.ED25519Signature)
	require.Equal(t, "foo", sig.Memo)
	require.Equal(t, "bar", string(sig.Data))
}

const rsaPrivateKey1024 = `-----BEGIN RSA PRIVATE KEY-----
MIICXQIBAAKBgQCgA3+iQ1/zYRcKAATz/y+KYAW0boh9VGEFFamlnhe2I2FuEty4
bFHxu9ntzIS5u1q8Ol49n9pgHF80G4scIKbWqR2M8m0c9YuNDejkXbW/Iqf2tZwk
jArlMFcRxgvePfjqZXUnUqpu0n8A1BNQ3uo5S1RsK9GvwbVvOcLutlzLgwIDAQAB
AoGAL/AcYs5whoeF0XckBL1kzr3pt56NwY5v6ogM5RMx411CKSn5ej7pZdRze6yT
7tjUXCPYa/niAH0/gGroCCs4EAlN/+xCAnF9SM6js4Gu4xMtTstasOyyKN/nlhUE
zrpbcTLr/cJtjXfZniajFmm4Urz7mzdlW5rULyAcZ5g/PNECQQDjZuXeR6qlxxRE
jAwKkou4zRuSu95hCJUf9W3val8I7CTkvyk75xilfwDnzquasRp14xdADHy81TW7
Wp437uVPAkEAtCMQ0YUWrsvftt4Hla5xefczykW8pQ/07FzeN6cN/ajgH3QWJxip
oXJZJ+P9XvFS60PMXhyE0iHjOfyr6X3RjQJBAItbzPV60A6GQVp8xQhZpLzdHc+/
yFmI6/LI8tVtR85tAXMZ34gxaL5LZd+pnSrQ7FlgkSgUPwFuXF5z+1Bl3CsCQBTC
qdCL1xZkFq9bnWIpzZgx3j0kll4rnZ2UAmRFk341dUcKuPbeh8Y8iHvpcaz8gQLu
OGJsRP52u1pWfXWWc40CQQCqwVesy8mZdV1JgglEsrtlvPcK0a/kVZQqPIGpthfV
D56486GwVTwyH6QCTD/ZxMficLzw+DpTXiRZd9UHyoBR
-----END RSA PRIVATE KEY-----`

func TestNewSigType(t *testing.T) {
	alice := AccountUrl("alice")

	block, _ := pem.Decode([]byte(rsaPrivateKey1024))
	aliceKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	require.NoError(t, err)

	cases := []struct {
		Version ExecutorVersion
		Ok      bool
	}{
		{ExecutorVersionV2Baikonur, false},
		{ExecutorVersionLatest, true},
	}

	for _, c := range cases {
		t.Run(c.Version.String(), func(t *testing.T) {
			// Initialize
			sim := NewSim(t,
				simulator.SimpleNetwork(t.Name(), 3, 1),
				simulator.GenesisWithVersion(GenesisTime, c.Version),
			)

			MakeIdentity(t, sim.DatabaseFor(alice), alice, x509.MarshalPKCS1PublicKey(&aliceKey.PublicKey))
			CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)

			// Submit
			env := build.Transaction().For(alice, "book", "1").
				BurnCredits(1).
				SignWith(alice, "book", "1").
				Version(1).Timestamp(1).
				Memo("foo").
				Metadata("bar").
				Type(SignatureTypeRsaSha256).
				PrivateKey(x509.MarshalPKCS1PrivateKey(aliceKey))

			if c.Ok {
				st := sim.BuildAndSubmitSuccessfully(env)
				sim.StepUntil(
					Txn(st[0].TxID).Succeeds())
			} else {
				st := sim.BuildAndSubmit(env)
				require.Error(t, st[1].AsError())
				require.ErrorIs(t, st[1].Error, errors.NotAllowed)
			}
		})
	}
}

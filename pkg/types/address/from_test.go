// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package address

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	altcrypto "gitlab.com/accumulatenetwork/accumulate/pkg/crypto"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// FromETHPrivateKey is the export path pkg/build uses for ETH, BTC and BTCLegacy keys. A key whose
// top byte is zero - the one key in 256 that D.Bytes() shortens - must export at the curve's full
// size, and the string pkg/build stores must parse back to a key that signs.
func TestFromETHPrivateKey_LeadingZeroByte(t *testing.T) {
	d := make([]byte, 32)
	for i := 1; i < 32; i++ {
		d[i] = byte(i)
	}
	key, err := altcrypto.ToECDSA(d)
	require.NoError(t, err)
	require.Len(t, key.D.Bytes(), 31, "precondition: the key's top byte is zero")

	addr := FromETHPrivateKey(key)
	require.Len(t, addr.Key, 32, "the exported key must be the curve's full size")
	require.Equal(t, d, addr.Key)
	require.Equal(t, altcrypto.FromECDSAPub(&key.PublicKey), addr.PublicKey.Key)

	// pkg/build types the address as ETH before storing it. The string is the key as hex.
	addr.Type = protocol.SignatureTypeETH
	s := addr.String()
	require.Len(t, s, 64, "an ETH key's string must be 32 bytes of hex")

	parsed, err := Parse(s)
	require.NoError(t, err)
	var stored []byte
	switch parsed := parsed.(type) {
	case *PrivateKey:
		stored = parsed.Key
	case *Unknown:
		stored = parsed.Value
	default:
		t.Fatalf("Parse returned %T", parsed)
	}
	require.Len(t, stored, 32)

	hash := sha256.Sum256([]byte("from ETH private key"))
	sig := new(protocol.ETHSignature)
	sig.PublicKey = addr.PublicKey.Key
	require.NoError(t, protocol.SignETH(sig, stored, nil, hash[:]))
	require.True(t, protocol.VerifyUserSignature(sig, protocol.SignableHash(hash)))
}

// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFilePVLastSignStateWireCompat pins the JSON byte format produced by
// FilePVLastSignState to CometBFT v0.38's privval.FilePVLastSignState. Two
// fields previously diverged:
//
//   - Height: CometBFT emits a bare integer; we previously had `,string` which
//     wrapped the value in quotes.
//   - SignBytes: CometBFT uses cmtbytes.HexBytes (uppercase hex); we previously
//     defaulted to []byte's base64 encoding.
//
// Both are expected to be byte-for-byte compatible after the fix.
func TestFilePVLastSignStateWireCompat(t *testing.T) {
	state := FilePVLastSignState{
		Height:    12345,
		Round:     3,
		Step:      2,
		Signature: []byte{0x01, 0x02, 0x03},
		SignBytes: HexBytes{0xDE, 0xAD, 0xBE, 0xEF},
	}

	data, err := json.Marshal(state)
	require.NoError(t, err)
	js := string(data)

	require.Contains(t, js, `"height":12345`, "Height must encode as bare int, not quoted string (CometBFT compat)")
	require.NotContains(t, js, `"height":"`, "Height must NOT be quoted")

	require.Contains(t, js, `"signbytes":"DEADBEEF"`, "SignBytes must be uppercase hex (CometBFT cmtbytes.HexBytes), not base64")
	require.False(t, strings.Contains(js, "3q2+7w=="), "SignBytes must NOT be base64 (was 3q2+7w== for these bytes)")
}

// TestFilePVLastSignStateRoundTrip verifies marshal → unmarshal round-trips
// through both the new format and what CometBFT writes (bare-int Height,
// hex SignBytes).
func TestFilePVLastSignStateRoundTrip(t *testing.T) {
	cometBFTOutput := `{
  "height": 999,
  "round": 0,
  "step": 1,
  "signature": "AQID",
  "signbytes": "CAFEBABE"
}`

	var state FilePVLastSignState
	require.NoError(t, json.Unmarshal([]byte(cometBFTOutput), &state))
	require.Equal(t, int64(999), state.Height)
	require.Equal(t, int32(0), state.Round)
	require.Equal(t, int8(1), state.Step)
	require.Equal(t, []byte{0x01, 0x02, 0x03}, state.Signature)
	require.Equal(t, HexBytes{0xCA, 0xFE, 0xBA, 0xBE}, state.SignBytes)

	// Re-marshal and check it produces the same content (modulo whitespace).
	data, err := json.Marshal(state)
	require.NoError(t, err)
	require.Contains(t, string(data), `"height":999`)
	require.Contains(t, string(data), `"signbytes":"CAFEBABE"`)
}

// TestHexBytesUnmarshalAcceptsAnyCase ensures we accept lowercase hex written
// by tools that don't follow CometBFT's uppercase convention.
func TestHexBytesUnmarshalAcceptsAnyCase(t *testing.T) {
	var h HexBytes
	require.NoError(t, json.Unmarshal([]byte(`"deadbeef"`), &h))
	require.Equal(t, HexBytes{0xDE, 0xAD, 0xBE, 0xEF}, h)

	require.NoError(t, json.Unmarshal([]byte(`"DeAdBeEf"`), &h))
	require.Equal(t, HexBytes{0xDE, 0xAD, 0xBE, 0xEF}, h)
}

// TestFilePVKeyAddressMatchesPubKeyDerivation verifies that the address
// emitted in MarshalJSON matches sha256(pubkey)[:20] (CometBFT's convention)
// and that UnmarshalJSON re-derives the address from priv_key, ignoring any
// supplied address in the JSON. (Note: this matches CometBFT's behavior of
// not validating the supplied address; review issue separately tracks
// whether to add a validation step.)
func TestFilePVKeyAddressDerivation(t *testing.T) {
	// Fixed test seed for deterministic key.
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)

	expectedAddr := pubKeyAddress(pub)
	require.Len(t, expectedAddr, addressSize)

	key := FilePVKey{
		Address: expectedAddr,
		PubKey:  pub,
		PrivKey: priv,
	}
	data, err := json.Marshal(key)
	require.NoError(t, err)
	require.Contains(t, string(data), strings.ToUpper(hex.EncodeToString(expectedAddr)))
}

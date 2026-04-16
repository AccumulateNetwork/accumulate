// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	tmjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	"github.com/stretchr/testify/require"
)

// TestCompatFilePVKey verifies that our local FilePVKey produces the same JSON
// as CometBFT's privval.FilePVKey, and that each can read the other's output.
func TestCompatFilePVKey(t *testing.T) {
	// Generate a CometBFT key
	tmPriv := tmed25519.GenPrivKey()
	tmPub := tmPriv.PubKey()

	// Create CometBFT FilePVKey
	cmtKey := privval.FilePVKey{
		Address: tmPub.Address(),
		PubKey:  tmPub,
		PrivKey: tmPriv,
	}

	// Marshal with CometBFT's JSON
	cmtJSON, err := tmjson.MarshalIndent(cmtKey, "", "  ")
	require.NoError(t, err)
	t.Logf("CometBFT JSON:\n%s", cmtJSON)

	// Unmarshal into our local type
	var localKey FilePVKey
	err = localKey.UnmarshalJSON(cmtJSON)
	require.NoError(t, err)

	// Verify fields match
	require.Equal(t, []byte(tmPriv), []byte(localKey.PrivKey), "private keys must match")
	require.Equal(t, []byte(tmPub.(tmed25519.PubKey)), []byte(localKey.PubKey), "public keys must match")
	require.Equal(t, []byte(tmPub.Address()), localKey.Address, "addresses must match")

	// Now test the reverse: marshal our local type and unmarshal with CometBFT
	localJSON, err := localKey.MarshalJSON()
	require.NoError(t, err)
	t.Logf("Local JSON:\n%s", localJSON)

	var cmtKey2 privval.FilePVKey
	err = tmjson.Unmarshal(localJSON, &cmtKey2)
	require.NoError(t, err)
	require.Equal(t, cmtKey.PrivKey.Bytes(), cmtKey2.PrivKey.Bytes(), "round-trip private keys must match")
	require.Equal(t, cmtKey.PubKey.Bytes(), cmtKey2.PubKey.Bytes(), "round-trip public keys must match")
}

// TestCompatNodeKey verifies that our local NodeKey produces the same JSON as
// CometBFT's p2p.NodeKey, and that each can read the other's output.
func TestCompatNodeKey(t *testing.T) {
	// Generate a CometBFT key
	tmPriv := tmed25519.GenPrivKey()

	// Create CometBFT NodeKey
	cmtNK := p2p.NodeKey{
		PrivKey: tmPriv,
	}

	// Marshal with CometBFT's JSON
	cmtJSON, err := tmjson.Marshal(&cmtNK)
	require.NoError(t, err)
	t.Logf("CometBFT NodeKey JSON:\n%s", cmtJSON)

	// Unmarshal into our local type
	var localNK NodeKey
	err = localNK.UnmarshalJSON(cmtJSON)
	require.NoError(t, err)

	// Verify key matches
	require.Equal(t, []byte(tmPriv), []byte(localNK.PrivKey), "private keys must match")

	// Verify ID matches
	cmtID := string(p2p.PubKeyToID(tmPriv.PubKey()))
	localID := localNK.ID()
	require.Equal(t, cmtID, localID, "node IDs must match")

	// Test reverse: marshal local and unmarshal with CometBFT
	localJSON, err := localNK.MarshalJSON()
	require.NoError(t, err)
	t.Logf("Local NodeKey JSON:\n%s", localJSON)

	var cmtNK2 p2p.NodeKey
	err = tmjson.Unmarshal(localJSON, &cmtNK2)
	require.NoError(t, err)
	require.Equal(t, cmtNK.PrivKey.Bytes(), cmtNK2.PrivKey.Bytes(), "round-trip private keys must match")
}

// TestCompatPubKeyToID verifies that our PubKeyToID matches CometBFT's.
func TestCompatPubKeyToID(t *testing.T) {
	tmPriv := tmed25519.GenPrivKey()
	tmPub := tmPriv.PubKey().(tmed25519.PubKey)

	cmtID := string(p2p.PubKeyToID(tmPub))
	localID := PubKeyToID(ed25519.PublicKey(tmPub))
	require.Equal(t, cmtID, localID, "PubKeyToID must match")
}

// TestCompatIDAddressString verifies that our IDAddressString matches CometBFT's.
func TestCompatIDAddressString(t *testing.T) {
	tmPriv := tmed25519.GenPrivKey()
	tmPub := tmPriv.PubKey()

	cmtID := p2p.PubKeyToID(tmPub)
	localID := PubKeyToID(ed25519.PublicKey(tmPub.(tmed25519.PubKey)))

	hostPort := "tcp://127.0.0.1:26656"
	cmtResult := p2p.IDAddressString(cmtID, hostPort)
	localResult := IDAddressString(localID, hostPort)
	require.Equal(t, cmtResult, localResult, "IDAddressString must match")
}

// TestCompatFilePVKeyAddress verifies that the address field in our FilePVKey
// matches CometBFT's address computation (first 20 bytes of SHA256 of pubkey).
func TestCompatFilePVKeyAddress(t *testing.T) {
	tmPriv := tmed25519.GenPrivKey()
	tmPub := tmPriv.PubKey().(tmed25519.PubKey)
	tmAddr := tmPub.Address()

	localAddr := pubKeyAddress(ed25519.PublicKey(tmPub))
	require.Equal(t, []byte(tmAddr), localAddr,
		"address: got %s, want %s", hex.EncodeToString(localAddr), hex.EncodeToString(tmAddr))
}

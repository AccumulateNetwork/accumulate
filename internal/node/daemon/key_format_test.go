// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	stded25519 "crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/p2p"
	"github.com/stretchr/testify/require"
)

// TestNodeKeyFormat tests the correct format for CometBFT node keys
// and documents what format is expected by the P2P initialization
func TestNodeKeyFormat(t *testing.T) {
	// Test key from our deployment script (32 bytes hex)
	keyHex := "d67aef864777361430c7b9137423ee5313d6a231a4c614885dc280b58d2314fd"
	keyBytes, err := hex.DecodeString(keyHex)
	require.NoError(t, err)
	require.Equal(t, 32, len(keyBytes), "ed25519 private key should be 32 bytes")

	// Create node key the way Accumulate does it
	nodeKey := p2p.NodeKey{
		PrivKey: tmed25519.PrivKey(keyBytes),
	}

	// Check what PrivKey.Bytes() returns (this is what causes the P2P error)
	privKeyBytes := nodeKey.PrivKey.Bytes()
	t.Logf("Original key length: %d bytes", len(keyBytes))
	t.Logf("PrivKey.Bytes() length: %d bytes", len(privKeyBytes))
	t.Logf("PrivKey.Bytes() hex: %x", privKeyBytes)

	// The P2P error says "bad private key length: 48", so let's see what's happening
	require.Equal(t, 32, len(privKeyBytes), "PrivKey.Bytes() should return 32 bytes for ed25519")

	// Test saving and loading the key to see the JSON format
	tempDir := t.TempDir()
	keyFile := filepath.Join(tempDir, "node_key.json")
	
	err = nodeKey.SaveAs(keyFile)
	require.NoError(t, err)

	// Read the saved file to see the format
	data, err := os.ReadFile(keyFile)
	require.NoError(t, err)
	t.Logf("Saved node key JSON: %s", string(data))

	// Parse the JSON to understand the structure
	var keyData struct {
		PrivKey struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"priv_key"`
	}
	err = json.Unmarshal(data, &keyData)
	require.NoError(t, err)

	t.Logf("Key type: %s", keyData.PrivKey.Type)
	t.Logf("Key value: %s", keyData.PrivKey.Value)

	// Check if the value is base64 or hex
	if decoded, err := base64.StdEncoding.DecodeString(keyData.PrivKey.Value); err == nil {
		t.Logf("Key value is base64, decoded length: %d bytes", len(decoded))
		t.Logf("Decoded key hex: %x", decoded)
	} else if decoded, err := hex.DecodeString(keyData.PrivKey.Value); err == nil {
		t.Logf("Key value is hex, decoded length: %d bytes", len(decoded))
	} else {
		t.Logf("Key value format unknown")
	}

	// Test loading the key back
	loadedKey, err := p2p.LoadNodeKey(keyFile)
	require.NoError(t, err)
	
	loadedPrivKeyBytes := loadedKey.PrivKey.Bytes()
	t.Logf("Loaded PrivKey.Bytes() length: %d bytes", len(loadedPrivKeyBytes))
	
	// Verify the loaded key matches the original
	require.Equal(t, privKeyBytes, loadedPrivKeyBytes, "Loaded key should match original")
}

// TestNodeKeyFromDeploymentScript tests the exact format our deployment script generates
func TestNodeKeyFromDeploymentScript(t *testing.T) {
	// Test the OLD format (hex-encoded) that causes the P2P error
	oldDeploymentKeyJSON := `{
  "priv_key": {
    "type": "tendermint/PrivKeyEd25519",
    "value": "d67aef864777361430c7b9137423ee5313d6a231a4c614885dc280b58d2314fd"
  }
}`

	// Write this to a temp file and try to load it
	tempDir := t.TempDir()
	oldKeyFile := filepath.Join(tempDir, "old_deployment_node_key.json")
	
	err := os.WriteFile(oldKeyFile, []byte(oldDeploymentKeyJSON), 0644)
	require.NoError(t, err)

	// Try to load it with CometBFT
	loadedKey, err := p2p.LoadNodeKey(oldKeyFile)
	if err != nil {
		t.Logf("Failed to load OLD deployment script key format: %v", err)
		t.Logf("This explains why P2P initialization fails")
	} else {
		privKeyBytes := loadedKey.PrivKey.Bytes()
		t.Logf("OLD format: PrivKey.Bytes() length: %d (should be 48, causing P2P error)", len(privKeyBytes))
		require.Equal(t, 48, len(privKeyBytes), "Old hex format should produce 48-byte key (the bug)")
	}

	// Test the NEW format (base64-encoded) that should work
	// Convert the same hex key to base64
	keyHex := "d67aef864777361430c7b9137423ee5313d6a231a4c614885dc280b58d2314fd"
	keyBytes, err := hex.DecodeString(keyHex)
	require.NoError(t, err)
	keyBase64 := base64.StdEncoding.EncodeToString(keyBytes)
	
	newDeploymentKeyJSON := fmt.Sprintf(`{
  "priv_key": {
    "type": "tendermint/PrivKeyEd25519",
    "value": "%s"
  }
}`, keyBase64)

	newKeyFile := filepath.Join(tempDir, "new_deployment_node_key.json")
	err = os.WriteFile(newKeyFile, []byte(newDeploymentKeyJSON), 0644)
	require.NoError(t, err)

	// Try to load the corrected format
	loadedNewKey, err := p2p.LoadNodeKey(newKeyFile)
	require.NoError(t, err, "NEW base64 format should load successfully")
	
	newPrivKeyBytes := loadedNewKey.PrivKey.Bytes()
	t.Logf("NEW format: PrivKey.Bytes() length: %d (should be 32, fixing P2P error)", len(newPrivKeyBytes))
	require.Equal(t, 32, len(newPrivKeyBytes), "New base64 format should produce 32-byte key")
	
	// Verify the key data is the same
	require.Equal(t, keyBytes, newPrivKeyBytes, "Key data should be preserved")
}

// TestP2PInitialization tests the exact P2P initialization that's failing in the daemon
func TestP2PInitialization(t *testing.T) {
	// Create a node key file with the corrected base64 format
	tempDir := t.TempDir()
	keyFile := filepath.Join(tempDir, "node_key.json")
	
	// Generate a proper base64-encoded key
	keyBytes := make([]byte, 32)
	_, err := rand.Read(keyBytes)
	require.NoError(t, err)
	
	keyBase64 := base64.StdEncoding.EncodeToString(keyBytes)
	keyJSON := fmt.Sprintf(`{
  "priv_key": {
    "type": "tendermint/PrivKeyEd25519",
    "value": "%s"
  }
}`, keyBase64)
	
	err = os.WriteFile(keyFile, []byte(keyJSON), 0644)
	require.NoError(t, err)
	
	// Load the key using CometBFT's LoadNodeKey (same as daemon does)
	nodeKey, err := p2p.LoadNodeKey(keyFile)
	require.NoError(t, err)
	
	privKeyBytes := nodeKey.PrivKey.Bytes()
	t.Logf("Loaded key length: %d bytes", len(privKeyBytes))
	t.Logf("Key type: %T", nodeKey.PrivKey)
	t.Logf("Key bytes hex: %x", privKeyBytes)
	
	// Test the exact conversion that the daemon does for P2P initialization
	// This is the line that fails: stded25519.PrivateKey(d.nodeKey.PrivKey.Bytes())
	// The issue might be that CometBFT returns a 32-byte seed, but standard ed25519 expects 64 bytes
	t.Logf("Attempting direct conversion (this should fail)...")
	
	// Try the direct conversion that the daemon does
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("Direct conversion failed as expected: %v", r)
			}
		}()
		p2pKey := stded25519.PrivateKey(privKeyBytes)
		t.Logf("Direct conversion succeeded, P2P key length: %d bytes", len(p2pKey))
	}()
	
	// Try the correct way: generate a proper 64-byte ed25519 private key from the 32-byte seed
	t.Logf("Attempting correct conversion using NewKeyFromSeed...")
	if len(privKeyBytes) == 32 {
		// CometBFT gives us a 32-byte seed, convert it to a 64-byte private key
		p2pKey := stded25519.NewKeyFromSeed(privKeyBytes)
		t.Logf("Correct conversion succeeded, P2P key length: %d bytes", len(p2pKey))
		
		// Test signing with the correct key
		testMessage := []byte("test message")
		signature := stded25519.Sign(p2pKey, testMessage)
		t.Logf("Signature length: %d bytes", len(signature))
		
		// Verify the signature
		pubKey := p2pKey.Public().(stded25519.PublicKey)
		valid := stded25519.Verify(pubKey, testMessage, signature)
		require.True(t, valid, "Signature should be valid")
	} else {
		t.Fatalf("Expected 32-byte seed, got %d bytes", len(privKeyBytes))
	}
}

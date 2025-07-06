package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func TestGenerateAndParseValidatorKey(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "validator-key-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test ADI URL
	testADI := "acc://test-validator.acme"
	
	// Validate ADI URL
	adiURL, err := url.Parse(testADI)
	if err != nil {
		t.Fatalf("Invalid ADI URL %q: %v", testADI, err)
	}

	// Generate Ed25519 key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Derive validator address from public key (first 20 bytes of SHA256 hash)
	hash := sha256.Sum256(pubKey)
	expectedAddress := hex.EncodeToString(hash[:20])

	// Create the private validator key structure
	pvKey := PrivValidatorKey{
		Address: expectedAddress,
		PubKey: struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		}{
			Type:  "tendermint/PubKeyEd25519",
			Value: base64.StdEncoding.EncodeToString(pubKey),
		},
		PrivKey: struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		}{
			Type:  "tendermint/PrivKeyEd25519",
			Value: base64.StdEncoding.EncodeToString(privKey),
		},
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(pvKey, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal private validator key: %v", err)
	}

	// Write to file
	keyFile := filepath.Join(tempDir, "priv_validator_key.json")
	err = os.WriteFile(keyFile, data, 0600)
	if err != nil {
		t.Fatalf("Failed to write private validator key file: %v", err)
	}

	// Verify file exists and has correct permissions
	fileInfo, err := os.Stat(keyFile)
	if err != nil {
		t.Fatalf("Failed to stat key file: %v", err)
	}
	if fileInfo.Mode().Perm() != 0600 {
		t.Errorf("Expected file permissions 0600, got %o", fileInfo.Mode().Perm())
	}

	// Parse the file back
	readData, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatalf("Failed to read key file: %v", err)
	}

	var parsedKey PrivValidatorKey
	err = json.Unmarshal(readData, &parsedKey)
	if err != nil {
		t.Fatalf("Failed to parse key file: %v", err)
	}

	// Verify the parsed key matches what we generated
	if parsedKey.Address != expectedAddress {
		t.Errorf("Address mismatch: expected %s, got %s", expectedAddress, parsedKey.Address)
	}

	if parsedKey.PubKey.Type != "tendermint/PubKeyEd25519" {
		t.Errorf("Public key type mismatch: expected tendermint/PubKeyEd25519, got %s", parsedKey.PubKey.Type)
	}

	if parsedKey.PrivKey.Type != "tendermint/PrivKeyEd25519" {
		t.Errorf("Private key type mismatch: expected tendermint/PrivKeyEd25519, got %s", parsedKey.PrivKey.Type)
	}

	// Decode and verify public key
	decodedPubKey, err := base64.StdEncoding.DecodeString(parsedKey.PubKey.Value)
	if err != nil {
		t.Fatalf("Failed to decode public key: %v", err)
	}

	if len(decodedPubKey) != ed25519.PublicKeySize {
		t.Errorf("Public key size mismatch: expected %d, got %d", ed25519.PublicKeySize, len(decodedPubKey))
	}

	// Verify the decoded public key matches original
	if !ed25519.PublicKey(decodedPubKey).Equal(pubKey) {
		t.Error("Decoded public key does not match original")
	}

	// Decode and verify private key
	decodedPrivKey, err := base64.StdEncoding.DecodeString(parsedKey.PrivKey.Value)
	if err != nil {
		t.Fatalf("Failed to decode private key: %v", err)
	}

	if len(decodedPrivKey) != ed25519.PrivateKeySize {
		t.Errorf("Private key size mismatch: expected %d, got %d", ed25519.PrivateKeySize, len(decodedPrivKey))
	}

	// Verify the decoded private key matches original
	if !ed25519.PrivateKey(decodedPrivKey).Equal(privKey) {
		t.Error("Decoded private key does not match original")
	}

	// Test key functionality by signing and verifying
	testMessage := []byte("test message for signing")
	signature := ed25519.Sign(ed25519.PrivateKey(decodedPrivKey), testMessage)
	
	if !ed25519.Verify(ed25519.PublicKey(decodedPubKey), testMessage, signature) {
		t.Error("Signature verification failed")
	}

	// Verify address derivation
	derivedHash := sha256.Sum256(decodedPubKey)
	derivedAddress := hex.EncodeToString(derivedHash[:20])
	
	if derivedAddress != parsedKey.Address {
		t.Errorf("Address derivation mismatch: expected %s, got %s", parsedKey.Address, derivedAddress)
	}

	t.Logf("Successfully generated and verified validator key for ADI: %s", adiURL.String())
	t.Logf("Address: %s", parsedKey.Address)
	t.Logf("Public Key: %s", parsedKey.PubKey.Value)
	t.Logf("Key file: %s", keyFile)
}

func TestGenerateKeyCommand(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "generate-key-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test the generateKey function directly
	testADI := "acc://test-node.acme"
	
	// Mock command and args
	args := []string{testADI, tempDir}
	
	err = generateKey(nil, args)
	if err != nil {
		t.Fatalf("generateKey failed: %v", err)
	}

	// Verify the key file was created
	keyFile := filepath.Join(tempDir, "priv_validator_key.json")
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		t.Fatal("Key file was not created")
	}

	// Parse and verify the generated key
	data, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatalf("Failed to read generated key file: %v", err)
	}

	var pvKey PrivValidatorKey
	err = json.Unmarshal(data, &pvKey)
	if err != nil {
		t.Fatalf("Failed to parse generated key file: %v", err)
	}

	// Verify structure
	if pvKey.Address == "" {
		t.Error("Generated key has empty address")
	}
	
	if pvKey.PubKey.Type != "tendermint/PubKeyEd25519" {
		t.Errorf("Generated key has wrong public key type: %s", pvKey.PubKey.Type)
	}
	
	if pvKey.PrivKey.Type != "tendermint/PrivKeyEd25519" {
		t.Errorf("Generated key has wrong private key type: %s", pvKey.PrivKey.Type)
	}

	// Verify key functionality
	pubKeyBytes, err := base64.StdEncoding.DecodeString(pvKey.PubKey.Value)
	if err != nil {
		t.Fatalf("Failed to decode generated public key: %v", err)
	}

	privKeyBytes, err := base64.StdEncoding.DecodeString(pvKey.PrivKey.Value)
	if err != nil {
		t.Fatalf("Failed to decode generated private key: %v", err)
	}

	// Test signing
	testMessage := []byte("test signing with generated key")
	signature := ed25519.Sign(ed25519.PrivateKey(privKeyBytes), testMessage)
	
	if !ed25519.Verify(ed25519.PublicKey(pubKeyBytes), testMessage, signature) {
		t.Error("Generated key failed signature verification")
	}

	t.Logf("Successfully tested generateKey command")
	t.Logf("Generated key file: %s", keyFile)
	t.Logf("Address: %s", pvKey.Address)
}

func TestUpdateKeyCommand(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "update-key-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testADI := "acc://test-validator.acme"
	
	// First generate a key
	args := []string{testADI, tempDir}
	err = generateKey(nil, args)
	if err != nil {
		t.Fatalf("Failed to generate key for test: %v", err)
	}

	// Create a test network configuration
	networkConfig := NetworkConfig{
		Globals: struct {
			Oracle struct {
				Price int `json:"price"`
			} `json:"oracle"`
			Globals struct{} `json:"globals"`
			Network struct {
				NetworkName string `json:"networkName"`
				Partitions []struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				} `json:"partitions"`
				Validators []struct {
					Operator   string `json:"operator"`
					PublicKey  string `json:"publicKey"`
					Partitions []struct {
						ID     string `json:"id"`
						Active bool   `json:"active"`
					} `json:"partitions"`
				} `json:"validators"`
			} `json:"network"`
		}{
			Oracle: struct {
				Price int `json:"price"`
			}{
				Price: 5000,
			},
			Globals: struct{}{},
			Network: struct {
				NetworkName string `json:"networkName"`
				Partitions []struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				} `json:"partitions"`
				Validators []struct {
					Operator   string `json:"operator"`
					PublicKey  string `json:"publicKey"`
					Partitions []struct {
						ID     string `json:"id"`
						Active bool   `json:"active"`
					} `json:"partitions"`
				} `json:"validators"`
			}{
				NetworkName: "test-network",
				Partitions: []struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				}{
					{ID: "Directory", Type: "directory"},
				},
				Validators: []struct {
					Operator   string `json:"operator"`
					PublicKey  string `json:"publicKey"`
					Partitions []struct {
						ID     string `json:"id"`
						Active bool   `json:"active"`
					} `json:"partitions"`
				}{
					{
						Operator:  testADI,
						PublicKey: "old-key-placeholder",
						Partitions: []struct {
							ID     string `json:"id"`
							Active bool   `json:"active"`
						}{
							{ID: "Directory", Active: true},
						},
					},
				},
			},
		},
	}

	// Write network config to file
	networkFile := filepath.Join(tempDir, "test-network.json")
	networkData, err := json.MarshalIndent(networkConfig, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal network config: %v", err)
	}

	err = os.WriteFile(networkFile, networkData, 0644)
	if err != nil {
		t.Fatalf("Failed to write network config: %v", err)
	}

	// Test the updateKey function
	updateArgs := []string{testADI, networkFile, tempDir}
	err = updateKey(nil, updateArgs)
	if err != nil {
		t.Fatalf("updateKey failed: %v", err)
	}

	// Verify the network config was updated
	updatedData, err := os.ReadFile(networkFile)
	if err != nil {
		t.Fatalf("Failed to read updated network config: %v", err)
	}

	var updatedConfig NetworkConfig
	err = json.Unmarshal(updatedData, &updatedConfig)
	if err != nil {
		t.Fatalf("Failed to parse updated network config: %v", err)
	}

	// Verify the public key was updated
	if len(updatedConfig.Globals.Network.Validators) != 1 {
		t.Fatalf("Expected 1 validator, got %d", len(updatedConfig.Globals.Network.Validators))
	}

	validator := updatedConfig.Globals.Network.Validators[0]
	if validator.PublicKey == "old-key-placeholder" {
		t.Error("Public key was not updated")
	}

	// Verify the updated key is valid hex
	_, err = hex.DecodeString(validator.PublicKey)
	if err != nil {
		t.Errorf("Updated public key is not valid hex: %v", err)
	}

	t.Logf("Successfully tested updateKey command")
	t.Logf("Updated validator: %s", validator.Operator)
	t.Logf("New public key: %s", validator.PublicKey)
}

func TestInvalidADI(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "invalid-adi-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test with truly invalid ADI URL (missing scheme)
	invalidADI := "://invalid-url"
	args := []string{invalidADI, tempDir}
	
	err = generateKey(nil, args)
	if err == nil {
		t.Error("Expected error for invalid ADI URL, but got none")
	} else {
		t.Logf("Correctly rejected invalid ADI: %v", err)
	}

	// Test with URL that doesn't have acc:// scheme
	invalidADI2 := "http://example.com"
	args2 := []string{invalidADI2, tempDir}
	
	err2 := generateKey(nil, args2)
	if err2 == nil {
		t.Error("Expected error for non-acc:// URL, but got none")
	} else {
		t.Logf("Correctly rejected non-acc URL: %v", err2)
	}

	// Test with empty ADI
	emptyADI := ""
	args3 := []string{emptyADI, tempDir}
	
	err3 := generateKey(nil, args3)
	if err3 == nil {
		t.Error("Expected error for empty ADI URL, but got none")
	} else {
		t.Logf("Correctly rejected empty ADI: %v", err3)
	}
}

func TestBadKeys(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "bad-keys-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testADI := "acc://test-validator.acme"
	networkFile := filepath.Join(tempDir, "test-network.json")

	// Create a test network configuration
	networkConfig := NetworkConfig{
		Globals: struct {
			Oracle struct {
				Price int `json:"price"`
			} `json:"oracle"`
			Globals struct{} `json:"globals"`
			Network struct {
				NetworkName string `json:"networkName"`
				Partitions []struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				} `json:"partitions"`
				Validators []struct {
					Operator   string `json:"operator"`
					PublicKey  string `json:"publicKey"`
					Partitions []struct {
						ID     string `json:"id"`
						Active bool   `json:"active"`
					} `json:"partitions"`
				} `json:"validators"`
			} `json:"network"`
		}{
			Oracle: struct {
				Price int `json:"price"`
			}{
				Price: 5000,
			},
			Globals: struct{}{},
			Network: struct {
				NetworkName string `json:"networkName"`
				Partitions []struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				} `json:"partitions"`
				Validators []struct {
					Operator   string `json:"operator"`
					PublicKey  string `json:"publicKey"`
					Partitions []struct {
						ID     string `json:"id"`
						Active bool   `json:"active"`
					} `json:"partitions"`
				} `json:"validators"`
			}{
				NetworkName: "test-network",
				Partitions: []struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				}{
					{ID: "Directory", Type: "directory"},
				},
				Validators: []struct {
					Operator   string `json:"operator"`
					PublicKey  string `json:"publicKey"`
					Partitions []struct {
						ID     string `json:"id"`
						Active bool   `json:"active"`
					} `json:"partitions"`
				}{
					{
						Operator:  testADI,
						PublicKey: "old-key-placeholder",
						Partitions: []struct {
							ID     string `json:"id"`
							Active bool   `json:"active"`
						}{
							{ID: "Directory", Active: true},
						},
					},
				},
			},
		},
	}

	// Write network config to file
	networkData, err := json.MarshalIndent(networkConfig, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal network config: %v", err)
	}

	err = os.WriteFile(networkFile, networkData, 0644)
	if err != nil {
		t.Fatalf("Failed to write network config: %v", err)
	}

	// Test 1: Corrupted JSON file
	t.Run("CorruptedJSON", func(t *testing.T) {
		corruptedKeyDir := filepath.Join(tempDir, "corrupted")
		err := os.MkdirAll(corruptedKeyDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create corrupted key dir: %v", err)
		}
		corruptedKeyFile := filepath.Join(corruptedKeyDir, "priv_validator_key.json")
		corruptedJSON := `{"address": "invalid", "pub_key": {"type": "tendermint/PubKeyEd25519", "value": "corrupted"}, "priv_key": {"type": "tendermint/PrivKeyEd25519", "value": "also-corrupted"}` // Missing closing brace
		err = os.WriteFile(corruptedKeyFile, []byte(corruptedJSON), 0600)
		if err != nil {
			t.Fatalf("Failed to write corrupted key file: %v", err)
		}

		args := []string{testADI, networkFile, corruptedKeyDir}
		err = updateKey(nil, args)
		if err == nil {
			t.Error("Expected error for corrupted JSON, but got none")
		} else {
			t.Logf("Correctly rejected corrupted JSON: %v", err)
		}
	})

	// Test 2: Invalid base64 public key
	t.Run("InvalidBase64PublicKey", func(t *testing.T) {
		badPubKeyDir := filepath.Join(tempDir, "bad_pub_key")
		err := os.MkdirAll(badPubKeyDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create bad pub key dir: %v", err)
		}
		badPubKeyFile := filepath.Join(badPubKeyDir, "priv_validator_key.json")
		badPubKeyJSON := `{
			"address": "1234567890abcdef1234567890abcdef12345678",
			"pub_key": {
				"type": "tendermint/PubKeyEd25519",
				"value": "invalid-base64-!@#$%"
			},
			"priv_key": {
				"type": "tendermint/PrivKeyEd25519",
				"value": "dGVzdC1wcml2YXRlLWtleS12YWx1ZS10aGF0LWlzLXZhbGlkLWJhc2U2NA=="
			}
		}`
		err = os.WriteFile(badPubKeyFile, []byte(badPubKeyJSON), 0600)
		if err != nil {
			t.Fatalf("Failed to write bad public key file: %v", err)
		}

		args := []string{testADI, networkFile, badPubKeyDir}
		err = updateKey(nil, args)
		if err == nil {
			t.Error("Expected error for invalid base64 public key, but got none")
		} else {
			t.Logf("Correctly rejected invalid base64 public key: %v", err)
		}
	})

	// Test 3: Wrong key size (not 32 bytes for Ed25519)
	t.Run("WrongKeySize", func(t *testing.T) {
		wrongSizeKeyDir := filepath.Join(tempDir, "wrong_size_key")
		err := os.MkdirAll(wrongSizeKeyDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create wrong size key dir: %v", err)
		}
		wrongSizeKeyFile := filepath.Join(wrongSizeKeyDir, "priv_validator_key.json")
		// Create a base64 string that decodes to wrong size (16 bytes instead of 32)
		shortKey := base64.StdEncoding.EncodeToString(make([]byte, 16))
		wrongSizeJSON := fmt.Sprintf(`{
			"address": "1234567890abcdef1234567890abcdef12345678",
			"pub_key": {
				"type": "tendermint/PubKeyEd25519",
				"value": "%s"
			},
			"priv_key": {
				"type": "tendermint/PrivKeyEd25519",
				"value": "dGVzdC1wcml2YXRlLWtleS12YWx1ZS10aGF0LWlzLXZhbGlkLWJhc2U2NA=="
			}
		}`, shortKey)
		err = os.WriteFile(wrongSizeKeyFile, []byte(wrongSizeJSON), 0600)
		if err != nil {
			t.Fatalf("Failed to write wrong size key file: %v", err)
		}

		args := []string{testADI, networkFile, wrongSizeKeyDir}
		err = updateKey(nil, args)
		if err == nil {
			t.Error("Expected error for wrong key size, but got none")
		} else {
			t.Logf("Correctly rejected wrong key size: %v", err)
		}
	})

	// Test 4: Missing key file
	t.Run("MissingKeyFile", func(t *testing.T) {
		// Try to update with a non-existent key directory
		nonExistentDir := filepath.Join(tempDir, "does-not-exist")
		args := []string{testADI, networkFile, nonExistentDir}
		err := updateKey(nil, args)
		if err == nil {
			t.Error("Expected error for missing key file, but got none")
		} else {
			t.Logf("Correctly rejected missing key file: %v", err)
		}
	})

	// Test 5: Wrong key type (not Ed25519)
	t.Run("WrongKeyType", func(t *testing.T) {
		wrongTypeKeyDir := filepath.Join(tempDir, "wrong_type_key")
		err := os.MkdirAll(wrongTypeKeyDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create wrong type key dir: %v", err)
		}
		wrongTypeKeyFile := filepath.Join(wrongTypeKeyDir, "priv_validator_key.json")
		wrongTypeJSON := `{
			"address": "1234567890abcdef1234567890abcdef12345678",
			"pub_key": {
				"type": "tendermint/PubKeySecp256k1",
				"value": "dGVzdC1wdWJsaWMta2V5LXZhbHVlLXRoYXQtaXMtdmFsaWQtYmFzZTY0"
			},
			"priv_key": {
				"type": "tendermint/PrivKeySecp256k1",
				"value": "dGVzdC1wcml2YXRlLWtleS12YWx1ZS10aGF0LWlzLXZhbGlkLWJhc2U2NA=="
			}
		}`
		err = os.WriteFile(wrongTypeKeyFile, []byte(wrongTypeJSON), 0600)
		if err != nil {
			t.Fatalf("Failed to write wrong type key file: %v", err)
		}

		args := []string{testADI, networkFile, wrongTypeKeyDir}
		err = updateKey(nil, args)
		if err == nil {
			t.Error("Expected error for wrong key type, but got none")
		} else {
			t.Logf("Correctly rejected wrong key type: %v", err)
		}
	})

	// Test 6: Empty key values
	t.Run("EmptyKeyValues", func(t *testing.T) {
		emptyKeyDir := filepath.Join(tempDir, "empty_key")
		err := os.MkdirAll(emptyKeyDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create empty key dir: %v", err)
		}
		emptyKeyFile := filepath.Join(emptyKeyDir, "priv_validator_key.json")
		emptyKeyJSON := `{
			"address": "1234567890abcdef1234567890abcdef12345678",
			"pub_key": {
				"type": "tendermint/PubKeyEd25519",
				"value": ""
			},
			"priv_key": {
				"type": "tendermint/PrivKeyEd25519",
				"value": ""
			}
		}`
		err = os.WriteFile(emptyKeyFile, []byte(emptyKeyJSON), 0600)
		if err != nil {
			t.Fatalf("Failed to write empty key file: %v", err)
		}

		args := []string{testADI, networkFile, emptyKeyDir}
		err = updateKey(nil, args)
		if err == nil {
			t.Error("Expected error for empty key values, but got none")
		} else {
			t.Logf("Correctly rejected empty key values: %v", err)
		}
	})

	// Test 7: Unreadable key file (permission denied)
	t.Run("UnreadableKeyFile", func(t *testing.T) {
		unreadableKeyDir := filepath.Join(tempDir, "unreadable_key")
		err := os.MkdirAll(unreadableKeyDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create unreadable key dir: %v", err)
		}
		unreadableKeyFile := filepath.Join(unreadableKeyDir, "priv_validator_key.json")
		validJSON := `{
			"address": "1234567890abcdef1234567890abcdef12345678",
			"pub_key": {
				"type": "tendermint/PubKeyEd25519",
				"value": "dGVzdC1wdWJsaWMta2V5LXZhbHVlLXRoYXQtaXMtdmFsaWQtYmFzZTY0"
			},
			"priv_key": {
				"type": "tendermint/PrivKeyEd25519",
				"value": "dGVzdC1wcml2YXRlLWtleS12YWx1ZS10aGF0LWlzLXZhbGlkLWJhc2U2NA=="
			}
		}`
		err = os.WriteFile(unreadableKeyFile, []byte(validJSON), 0000) // No read permissions
		if err != nil {
			t.Fatalf("Failed to write unreadable key file: %v", err)
		}
		defer os.Chmod(unreadableKeyFile, 0644) // Restore permissions for cleanup

		args := []string{testADI, networkFile, unreadableKeyDir}
		err = updateKey(nil, args)
		if err == nil {
			t.Error("Expected error for unreadable key file, but got none")
		} else {
			t.Logf("Correctly rejected unreadable key file: %v", err)
		}
	})
}

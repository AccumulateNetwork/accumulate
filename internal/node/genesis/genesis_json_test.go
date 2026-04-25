// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package genesis

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// validPubKey returns a deterministic 32-byte ed25519 public key for tests.
func validPubKey(seedByte byte) pubKeyJSON {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = seedByte
	}
	priv := ed25519.NewKeyFromSeed(seed)
	return pubKeyJSON(priv.Public().(ed25519.PublicKey))
}

func validGenesis() GenesisDocJSON {
	pk := validPubKey(1)
	addr := sha256.Sum256(pk)
	return GenesisDocJSON{
		ChainID:       "accumulate-test",
		InitialHeight: 1,
		GenesisTime:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Validators: []GenesisValidatorJSON{
			{
				Address: hexBytes(addr[:20]),
				PubKey:  pk,
				Power:   100,
				Name:    "v1",
			},
		},
	}
}

func TestValidateAndComplete_Success(t *testing.T) {
	doc := validGenesis()
	require.NoError(t, doc.ValidateAndComplete())
	require.NotNil(t, doc.ConsensusParams, "should fill in default consensus params")
	require.Equal(t, int64(22020096), doc.ConsensusParams.Block.MaxBytes)
}

func TestValidateAndComplete_EmptyChainID(t *testing.T) {
	doc := validGenesis()
	doc.ChainID = ""
	err := doc.ValidateAndComplete()
	require.Error(t, err)
	require.Contains(t, err.Error(), "chain_id")
}

func TestValidateAndComplete_ChainIDTooLong(t *testing.T) {
	doc := validGenesis()
	doc.ChainID = strings.Repeat("x", MaxChainIDLen+1)
	err := doc.ValidateAndComplete()
	require.Error(t, err)
	require.Contains(t, err.Error(), "too long")
}

func TestValidateAndComplete_NegativeInitialHeight(t *testing.T) {
	doc := validGenesis()
	doc.InitialHeight = -5
	err := doc.ValidateAndComplete()
	require.Error(t, err)
	require.Contains(t, err.Error(), "initial_height cannot be negative")
}

func TestValidateAndComplete_ZeroInitialHeightDefaults(t *testing.T) {
	doc := validGenesis()
	doc.InitialHeight = 0
	require.NoError(t, doc.ValidateAndComplete())
	require.Equal(t, int64(1), doc.InitialHeight, "initial_height=0 should normalize to 1")
}

func TestValidateAndComplete_ZeroValidatorPower(t *testing.T) {
	doc := validGenesis()
	doc.Validators[0].Power = 0
	err := doc.ValidateAndComplete()
	require.Error(t, err)
	require.Contains(t, err.Error(), "voting power cannot be zero")
}

func TestValidateAndComplete_AddressMismatch(t *testing.T) {
	doc := validGenesis()
	// Replace address with a wrong one (different pubkey's address).
	wrongPK := validPubKey(99)
	wrongAddr := sha256.Sum256(wrongPK)
	doc.Validators[0].Address = hexBytes(wrongAddr[:20])
	err := doc.ValidateAndComplete()
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match pub_key-derived address")
}

func TestValidateAndComplete_EmptyAddressFilled(t *testing.T) {
	doc := validGenesis()
	pk := doc.Validators[0].PubKey
	doc.Validators[0].Address = nil
	require.NoError(t, doc.ValidateAndComplete())
	expected := sha256.Sum256(pk)
	require.Equal(t, hexBytes(expected[:20]), doc.Validators[0].Address,
		"empty address should be filled with sha256(pubkey)[:20]")
}

func TestValidateAndComplete_PubKeyWrongSize(t *testing.T) {
	doc := validGenesis()
	doc.Validators[0].PubKey = pubKeyJSON{0xAB, 0xCD}
	err := doc.ValidateAndComplete()
	require.Error(t, err)
	require.Contains(t, err.Error(), "pub_key must be 32 bytes")
}

func TestValidateAndComplete_ZeroGenesisTimeDefaults(t *testing.T) {
	doc := validGenesis()
	doc.GenesisTime = time.Time{}
	before := time.Now().UTC()
	require.NoError(t, doc.ValidateAndComplete())
	after := time.Now().UTC()
	require.False(t, doc.GenesisTime.IsZero(), "zero GenesisTime should default to time.Now()")
	require.True(t, !doc.GenesisTime.Before(before) && !doc.GenesisTime.After(after.Add(time.Second)),
		"defaulted GenesisTime should be near now")
}

func TestValidateAndComplete_ConsensusParamsValidated(t *testing.T) {
	doc := validGenesis()
	doc.ConsensusParams = &ConsensusParamsJSON{
		Block: BlockParamsJSON{MaxBytes: 0, MaxGas: -1},
	}
	err := doc.ValidateAndComplete()
	require.Error(t, err)
	require.Contains(t, err.Error(), "block.max_bytes cannot be 0")
}

// TestGenesisDocFromFile_RejectsInvalid verifies the file loader runs
// ValidateAndComplete (the previous implementation skipped validation,
// silently accepting malformed genesis).
func TestGenesisDocFromFile_RejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genesis.json")

	bad := validGenesis()
	bad.ChainID = "" // invalid
	data, err := json.Marshal(bad)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0644))

	_, err = GenesisDocFromFile(path)
	require.Error(t, err, "GenesisDocFromFile must reject genesis with empty chain_id")
	require.Contains(t, err.Error(), "chain_id")
}

func TestGenesisDocFromFile_AcceptsValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genesis.json")

	good := validGenesis()
	data, err := json.Marshal(good)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0644))

	doc, err := GenesisDocFromFile(path)
	require.NoError(t, err)
	require.Equal(t, "accumulate-test", doc.ChainID)
}

// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build ignore
// +build ignore

// This test verifies wire-format compatibility with CometBFT genesis types.
// Run manually with cometbft in go.mod.

package genesis

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/cometbft/cometbft/crypto/ed25519"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/require"
)

// TestCompatGenesisDocJSON verifies our GenesisDocJSON produces JSON that
// matches CometBFT's types.GenesisDoc when marshaled with cmtjson.
func TestCompatGenesisDocJSON(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	key := make(ed25519.PubKey, ed25519.PubKeySize)
	for i := range key {
		key[i] = byte(i + 1)
	}

	// Create CometBFT genesis doc
	cmtDoc := types.GenesisDoc{
		GenesisTime:   now,
		ChainID:       "test.BVN0",
		InitialHeight: 10,
		ConsensusParams: &types.ConsensusParams{
			Block:     types.BlockParams{MaxBytes: 22020096, MaxGas: -1},
			Evidence:  types.EvidenceParams{MaxAgeNumBlocks: 100000, MaxAgeDuration: 48 * time.Hour, MaxBytes: 1048576},
			Validator: types.ValidatorParams{PubKeyTypes: []string{"ed25519"}},
			Version:   types.VersionParams{App: 1},
			ABCI:      types.ABCIParams{VoteExtensionsEnableHeight: 0},
		},
		Validators: []types.GenesisValidator{
			{
				Address: key.Address(),
				PubKey:  key,
				Power:   1,
				Name:    "validator-0",
			},
		},
		AppHash:  []byte{0xDE, 0xAD, 0xBE, 0xEF},
		AppState: json.RawMessage("null"),
	}

	// Marshal with CometBFT's amino JSON
	cmtJSON, err := cmtjson.MarshalIndent(cmtDoc, "", "  ")
	require.NoError(t, err)

	// Create equivalent local genesis doc
	localDoc := GenesisDocJSON{
		GenesisTime:   now,
		ChainID:       "test.BVN0",
		InitialHeight: 10,
		ConsensusParams: &ConsensusParamsJSON{
			Block:     BlockParamsJSON{MaxBytes: 22020096, MaxGas: -1},
			Evidence:  EvidenceParamsJSON{MaxAgeNumBlocks: 100000, MaxAgeDuration: 48 * time.Hour, MaxBytes: 1048576},
			Validator: ValidatorParamsJSON{PubKeyTypes: []string{"ed25519"}},
			Version:   VersionParamsJSON{App: 1},
			ABCI:      ABCIParamsJSON{VoteExtensionsEnableHeight: 0},
		},
		Validators: []GenesisValidatorJSON{
			{
				Address: hexBytes(key.Address()),
				PubKey:  pubKeyJSON(key),
				Power:   1,
				Name:    "validator-0",
			},
		},
		AppHash:  hexBytes{0xDE, 0xAD, 0xBE, 0xEF},
		AppState: json.RawMessage("null"),
	}

	// Marshal with standard JSON
	localJSON, err := json.MarshalIndent(localDoc, "", "  ")
	require.NoError(t, err)

	// Verify our local doc marshals to valid JSON
	require.NotEmpty(t, localJSON)

	// Parse both into generic maps and compare structural fields
	// Note: amino JSON encodes int64 as strings, standard JSON as numbers,
	// so we compare string fields and structural elements individually.
	var cmtMap, localMap map[string]any
	require.NoError(t, json.Unmarshal(cmtJSON, &cmtMap))
	require.NoError(t, json.Unmarshal(localJSON, &localMap))

	// String fields match exactly
	require.Equal(t, cmtMap["chain_id"], localMap["chain_id"])
	require.Equal(t, cmtMap["app_hash"], localMap["app_hash"])

	// Validator pubkey format matches (amino JSON type/value wrapper)
	cmtVals := cmtMap["validators"].([]any)
	localVals := localMap["validators"].([]any)
	require.Equal(t, len(cmtVals), len(localVals))

	cmtVal0 := cmtVals[0].(map[string]any)
	localVal0 := localVals[0].(map[string]any)
	require.Equal(t, cmtVal0["address"], localVal0["address"])
	require.Equal(t, cmtVal0["name"], localVal0["name"])

	cmtPK := cmtVal0["pub_key"].(map[string]any)
	localPK := localVal0["pub_key"].(map[string]any)
	require.Equal(t, cmtPK["type"], localPK["type"])
	require.Equal(t, cmtPK["value"], localPK["value"])

	// Verify our local doc round-trips through our own JSON
	var fromLocal GenesisDocJSON
	require.NoError(t, json.Unmarshal(localJSON, &fromLocal))
	require.Equal(t, "test.BVN0", fromLocal.ChainID)
	require.Equal(t, int64(10), fromLocal.InitialHeight)
	require.Equal(t, []byte{0xDE, 0xAD, 0xBE, 0xEF}, []byte(fromLocal.AppHash))
	require.Equal(t, 1, len(fromLocal.Validators))
	require.Equal(t, "validator-0", fromLocal.Validators[0].Name)
	require.Equal(t, []byte(key), []byte(fromLocal.Validators[0].PubKey))
	require.Equal(t, []byte(key.Address()), []byte(fromLocal.Validators[0].Address))
}

// TestCompatGenesisDocJSONRoundTrip verifies that marshaling our GenesisDocJSON
// and unmarshaling it back produces the same values.
func TestCompatGenesisDocJSONRoundTrip(t *testing.T) {
	doc := GenesisDocJSON{
		GenesisTime:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		ChainID:       "mynet.DN",
		InitialHeight: 1,
		AppHash:       hexBytes{0x01, 0x02, 0x03},
		AppState:      json.RawMessage(`{"key":"value"}`),
	}

	data, err := json.Marshal(doc)
	require.NoError(t, err)

	var roundTrip GenesisDocJSON
	require.NoError(t, json.Unmarshal(data, &roundTrip))

	require.Equal(t, doc.ChainID, roundTrip.ChainID)
	require.Equal(t, doc.InitialHeight, roundTrip.InitialHeight)
	require.Equal(t, []byte(doc.AppHash), []byte(roundTrip.AppHash))
	require.Equal(t, string(doc.AppState), string(roundTrip.AppState))
}

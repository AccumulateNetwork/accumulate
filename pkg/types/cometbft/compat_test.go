// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build ignore
// +build ignore

// This test verifies wire-format compatibility with CometBFT types.
// Run manually: go test -tags="" -run TestCompat -count=1 ./pkg/types/cometbft/
// (requires adding cometbft back to go.mod temporarily)

package cometbft

import (
	"testing"
	"time"

	gogoproto "github.com/cosmos/gogoproto/proto"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	tmtypes "github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/require"
)

// TestCompatConsensusParams verifies that our ConsensusParams produces the same
// protobuf encoding as CometBFT's types.ConsensusParams.
func TestCompatConsensusParams(t *testing.T) {
	// Create CometBFT ConsensusParams with known values
	cmtParams := tmtypes.ConsensusParams{
		Block: tmtypes.BlockParams{
			MaxBytes: 22020096,
			MaxGas:   5000000,
		},
		Evidence: tmtypes.EvidenceParams{
			MaxAgeNumBlocks: 100000,
			MaxAgeDuration:  48 * time.Hour,
			MaxBytes:        1048576,
		},
		Validator: tmtypes.ValidatorParams{
			PubKeyTypes: []string{"ed25519"},
		},
		Version: tmtypes.VersionParams{
			App: 42,
		},
		ABCI: tmtypes.ABCIParams{
			VoteExtensionsEnableHeight: 100,
		},
	}

	// Marshal CometBFT params to protobuf via gogo-proto
	cmtProto := cmtParams.ToProto()
	cmtBytes, err := gogoproto.Marshal(&cmtProto)
	require.NoError(t, err)

	// Create our local params with same values
	localParams := ConsensusParams{
		Block: BlockParams{
			MaxBytes: 22020096,
			MaxGas:   5000000,
		},
		Evidence: EvidenceParams{
			MaxAgeNumBlocks: 100000,
			MaxAgeDuration:  48 * time.Hour,
			MaxBytes:        1048576,
		},
		Validator: ValidatorParams{
			PubKeyTypes: []string{"ed25519"},
		},
		Version: VersionParams{
			App: 42,
		},
		ABCI: ABCIParams{
			VoteExtensionsEnableHeight: 100,
		},
	}

	// Marshal our local params
	localBytes, err := localParams.MarshalBinary()
	require.NoError(t, err)

	// Verify the bytes match
	require.Equal(t, cmtBytes, localBytes, "protobuf encoding must match CometBFT")

	// Unmarshal CometBFT bytes into our type
	var roundTrip ConsensusParams
	require.NoError(t, roundTrip.UnmarshalBinary(cmtBytes))
	require.Equal(t, localParams, roundTrip)
}

// TestCompatConsensusParamsDefaults verifies default params match.
func TestCompatConsensusParamsDefaults(t *testing.T) {
	cmtDefaults := tmtypes.DefaultConsensusParams()
	localDefaults := DefaultConsensusParams()

	require.Equal(t, cmtDefaults.Block.MaxBytes, localDefaults.Block.MaxBytes)
	require.Equal(t, cmtDefaults.Block.MaxGas, localDefaults.Block.MaxGas)
	require.Equal(t, cmtDefaults.Evidence.MaxAgeNumBlocks, localDefaults.Evidence.MaxAgeNumBlocks)
	require.Equal(t, cmtDefaults.Evidence.MaxAgeDuration, localDefaults.Evidence.MaxAgeDuration)
	require.Equal(t, cmtDefaults.Evidence.MaxBytes, localDefaults.Evidence.MaxBytes)
	require.Equal(t, cmtDefaults.Validator.PubKeyTypes, localDefaults.Validator.PubKeyTypes)
}

// TestCompatConsensusParamsRoundTrip verifies CometBFT bytes can be decoded
// by our type and re-encoded to the same bytes.
func TestCompatConsensusParamsRoundTrip(t *testing.T) {
	cmtParams := tmtypes.ConsensusParams{
		Block:     tmtypes.BlockParams{MaxBytes: 1048576, MaxGas: -1},
		Evidence:  tmtypes.EvidenceParams{MaxAgeNumBlocks: 50000, MaxAgeDuration: 24 * time.Hour, MaxBytes: 524288},
		Validator: tmtypes.ValidatorParams{PubKeyTypes: []string{"ed25519", "secp256k1"}},
		Version:   tmtypes.VersionParams{App: 7},
		ABCI:      tmtypes.ABCIParams{VoteExtensionsEnableHeight: 0},
	}

	cmtProto := cmtParams.ToProto()
	cmtBytes, err := gogoproto.Marshal(&cmtProto)
	require.NoError(t, err)

	// Decode with our type
	var local ConsensusParams
	require.NoError(t, local.UnmarshalBinary(cmtBytes))

	// Verify field values
	require.Equal(t, int64(1048576), local.Block.MaxBytes)
	require.Equal(t, int64(-1), local.Block.MaxGas)
	require.Equal(t, int64(50000), local.Evidence.MaxAgeNumBlocks)
	require.Equal(t, 24*time.Hour, local.Evidence.MaxAgeDuration)
	require.Equal(t, int64(524288), local.Evidence.MaxBytes)
	require.Equal(t, []string{"ed25519", "secp256k1"}, local.Validator.PubKeyTypes)
	require.Equal(t, uint64(7), local.Version.App)
	require.Equal(t, int64(0), local.ABCI.VoteExtensionsEnableHeight)

	// Re-encode and decode again to verify semantic round-trip
	// Note: byte-level encoding may differ for zero-value fields (gogo-proto
	// emits empty submessages, we omit them), but decoded values must match.
	reEncoded, err := local.MarshalBinary()
	require.NoError(t, err)

	var roundTrip ConsensusParams
	require.NoError(t, roundTrip.UnmarshalBinary(reEncoded))
	require.Equal(t, local, roundTrip, "semantic round-trip must be lossless")
}

// TestCompatBlock verifies our Block type roundtrips CometBFT block protobuf.
func TestCompatBlock(t *testing.T) {
	// Create a CometBFT block protobuf
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	cmtBlock := cmtproto.Block{
		Header: cmtproto.Header{
			ChainID: "test.BVN0",
			Height:  42,
			Time:    now,
		},
	}

	cmtBytes, err := gogoproto.Marshal(&cmtBlock)
	require.NoError(t, err)

	// Decode with our type
	var local Block
	require.NoError(t, local.UnmarshalBinary(cmtBytes))

	require.Equal(t, "test.BVN0", local.ChainID)
	require.Equal(t, int64(42), local.Height)
	require.True(t, now.Equal(local.Time), "time must match")

	// Re-encode should be lossless (rawProto preserved)
	reEncoded, err := local.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, cmtBytes, reEncoded, "block round-trip must be lossless")
}

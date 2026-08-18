// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package cometbft

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestConsensusParamsAlwaysEmitsAllSubFields pins the proto wire output for
// ConsensusParams so that every sub-field tag is present, even when its
// inner content is empty. CometBFT's gogoproto-generated marshaler emits
// `tag, len=0` for non-nil empty sub-messages; matching that behavior is the
// "byte-identical" claim of the MR description. Previously the marshaler
// skipped a sub-field tag when its inner bytes were empty, producing
// shorter (and CometBFT-incompatible) output for partially-populated
// ConsensusParams.
func TestConsensusParamsAlwaysEmitsAllSubFields(t *testing.T) {
	// All-zero ConsensusParams. CometBFT proto3 with non-nil sub-message
	// pointers would emit five `tag, len=0` pairs (10 bytes).
	params := &ConsensusParams{}
	data, err := params.MarshalBinary()
	require.NoError(t, err)
	// 0a 00 = field 1 (block) wire-type=2, len=0
	// 12 00 = field 2 (evidence)
	// 1a 00 = field 3 (validator)
	// 22 00 = field 4 (version)
	// 2a 00 = field 5 (abci)
	require.Equal(t, "0a0012001a0022002a00", hex.EncodeToString(data),
		"empty ConsensusParams must emit tags 1-5 each with len=0")
}

// TestConsensusParamsDefaultsRoundTrip checks that DefaultConsensusParams()
// produces a binary form that round-trips through Marshal/Unmarshal exactly.
func TestConsensusParamsDefaultsRoundTrip(t *testing.T) {
	orig := DefaultConsensusParams()
	data, err := orig.MarshalBinary()
	require.NoError(t, err)

	got := new(ConsensusParams)
	require.NoError(t, got.UnmarshalBinary(data))

	require.Equal(t, orig.Block.MaxBytes, got.Block.MaxBytes)
	require.Equal(t, orig.Block.MaxGas, got.Block.MaxGas)
	require.Equal(t, orig.Evidence.MaxAgeNumBlocks, got.Evidence.MaxAgeNumBlocks)
	require.Equal(t, orig.Evidence.MaxAgeDuration, got.Evidence.MaxAgeDuration)
	require.Equal(t, orig.Evidence.MaxBytes, got.Evidence.MaxBytes)
	require.Equal(t, orig.Validator.PubKeyTypes, got.Validator.PubKeyTypes)
	require.Equal(t, orig.Version.App, got.Version.App)
	require.Equal(t, orig.ABCI.VoteExtensionsEnableHeight, got.ABCI.VoteExtensionsEnableHeight)
}

// TestConsensusParamsRoundTripWithABCIHeight ensures the full proto wire
// covers all five sub-fields including ABCI vote extensions.
func TestConsensusParamsRoundTripWithABCIHeight(t *testing.T) {
	orig := &ConsensusParams{
		Block: BlockParams{
			MaxBytes: 1 << 20,
			MaxGas:   -1,
		},
		Evidence: EvidenceParams{
			MaxAgeNumBlocks: 50,
			MaxAgeDuration:  3 * time.Hour,
			MaxBytes:        4096,
		},
		Validator: ValidatorParams{
			PubKeyTypes: []string{"ed25519", "secp256k1"},
		},
		Version: VersionParams{App: 7},
		ABCI:    ABCIParams{VoteExtensionsEnableHeight: 9999},
	}

	data, err := orig.MarshalBinary()
	require.NoError(t, err)

	got := new(ConsensusParams)
	require.NoError(t, got.UnmarshalBinary(data))
	require.Equal(t, orig.Block, got.Block)
	require.Equal(t, orig.Evidence, got.Evidence)
	require.Equal(t, orig.Validator, got.Validator)
	require.Equal(t, orig.Version, got.Version)
	require.Equal(t, orig.ABCI, got.ABCI)
}

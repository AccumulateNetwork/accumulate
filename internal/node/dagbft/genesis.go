// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package dagbft provides integration between DAG-BFT consensus and the
// Accumulate node infrastructure.
package dagbft

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"log/slog"

	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	ioutil "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/genesis"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// GenesisConfig holds configuration for DAG-BFT genesis initialization.
type GenesisConfig struct {
	// Partition is the network partition identifier.
	Partition string

	// NetworkID is the network identifier (e.g., "mainnet", "testnet").
	NetworkID string

	// Validators contains the validator information including keys and stakes.
	Validators []genesis.ValidatorInfo

	// SnapshotData contains the initial state snapshot bytes, if any.
	SnapshotData []byte
}

// GenesisState represents the initialized genesis state for DAG-BFT.
type GenesisState struct {
	// Committee is the genesis committee.
	Committee *types.Committee

	// GenesisCerts contains the genesis certificates for all validators.
	GenesisCerts []*types.Certificate

	// InitialRound is the starting round (always 1).
	InitialRound types.Round

	// StateHash is the hash of the initial state (from snapshot).
	StateHash [32]byte
}

// InitializeGenesis creates the genesis state for DAG-BFT consensus.
// This includes creating genesis certificates and optionally loading
// initial state from a snapshot.
func InitializeGenesis(cfg GenesisConfig) (*GenesisState, error) {
	if cfg.Partition == "" {
		return nil, errors.New("partition is required")
	}
	if len(cfg.Validators) == 0 {
		return nil, errors.New("at least one validator is required")
	}

	slog.Info("Initializing DAG-BFT genesis",
		"partition", cfg.Partition,
		"network", cfg.NetworkID,
		"validators", len(cfg.Validators))

	// Create genesis certificates and committee
	result, err := genesis.InitGenesis(cfg.Validators)
	if err != nil {
		return nil, fmt.Errorf("init genesis: %w", err)
	}

	state := &GenesisState{
		Committee:    result.Committee,
		GenesisCerts: result.GenesisCerts,
		InitialRound: result.InitialRound,
	}

	// If snapshot data is provided, extract the state hash
	if len(cfg.SnapshotData) > 0 {
		hash, err := extractStateHash(cfg.SnapshotData, cfg.Partition)
		if err != nil {
			return nil, fmt.Errorf("extract state hash: %w", err)
		}
		state.StateHash = hash
	}

	slog.Info("DAG-BFT genesis initialized",
		"committee_size", result.Committee.Len(),
		"epoch", result.Committee.Epoch,
		"initial_round", result.InitialRound)

	return state, nil
}

// extractStateHash reads the state hash from snapshot data.
// The partition parameter is reserved for future use.
func extractStateHash(data []byte, _ string) ([32]byte, error) {
	var hash [32]byte

	reader := ioutil.NewBuffer(data)
	sr, err := snapshot.Open(reader)
	if err != nil {
		return hash, fmt.Errorf("open snapshot: %w", err)
	}

	// The Reader's Header contains the root hash
	if sr.Header == nil {
		return hash, errors.New("snapshot has no header")
	}

	hash = sr.Header.RootHash
	return hash, nil
}

// ValidatorInfoFromKeys creates validator info from private keys with
// the specified stake. This is useful for testing and simple deployments.
func ValidatorInfoFromKeys(keys []ed25519.PrivateKey, stake uint64) []genesis.ValidatorInfo {
	if stake == 0 {
		stake = 1
	}

	validators := make([]genesis.ValidatorInfo, len(keys))
	for i, key := range keys {
		validators[i] = genesis.ValidatorInfo{
			PublicKey:  key.Public().(ed25519.PublicKey),
			PrivateKey: key,
			Stake:      stake,
		}
	}
	return validators
}

// ValidatorInfoFromConfig extracts validator information from the node
// configuration. This reads validators from the global network configuration.
func ValidatorInfoFromConfig(networkCfg *protocol.NetworkDefinition, partition string, getPrivateKey func(pubKey []byte) (ed25519.PrivateKey, bool)) ([]genesis.ValidatorInfo, error) {
	var validators []genesis.ValidatorInfo

	for _, v := range networkCfg.Validators {
		if !v.IsActiveOn(partition) {
			continue
		}

		// Get the private key for this validator
		privKey, ok := getPrivateKey(v.PublicKey)
		if !ok {
			// Skip validators we don't have private keys for
			// (this is expected for remote validators)
			continue
		}

		validators = append(validators, genesis.ValidatorInfo{
			PublicKey:  v.PublicKey,
			PrivateKey: privKey,
			Stake:      1, // Default to unit stake; can be enhanced with power from config
		})
	}

	if len(validators) == 0 {
		return nil, errors.New("no validators found for partition")
	}

	return validators, nil
}

// VerifyGenesisState verifies that the genesis state is valid.
// This checks that all certificates are valid and properly signed.
func VerifyGenesisState(state *GenesisState) error {
	if state == nil {
		return errors.New("genesis state is nil")
	}
	if state.Committee == nil {
		return errors.New("committee is nil")
	}
	if len(state.GenesisCerts) == 0 {
		return errors.New("no genesis certificates")
	}

	// Verify committee
	if err := state.Committee.Validate(); err != nil {
		return fmt.Errorf("invalid committee: %w", err)
	}

	// Verify each certificate
	for i, cert := range state.GenesisCerts {
		if err := cert.Verify(state.Committee); err != nil {
			return fmt.Errorf("invalid genesis cert %d: %w", i, err)
		}
		if cert.Round() != 0 {
			return fmt.Errorf("genesis cert %d has non-zero round", i)
		}
	}

	// Verify we have a certificate for each validator
	if len(state.GenesisCerts) != state.Committee.Len() {
		return fmt.Errorf("expected %d genesis certs, got %d",
			state.Committee.Len(), len(state.GenesisCerts))
	}

	// Verify initial round
	if state.InitialRound != 1 {
		return fmt.Errorf("initial round should be 1, got %d", state.InitialRound)
	}

	return nil
}

// CreateNetworkURL creates a network URL for the given partition.
func CreateNetworkURL(partition string) config.NetworkUrl {
	return config.NetworkUrl{URL: protocol.PartitionUrl(partition)}
}

// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package genesis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tm "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/crypto/ed25519"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/node"
	"github.com/cometbft/cometbft/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/cometbft"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func ConvertJsonToSnapshot(doc *types.GenesisDoc) ([]byte, error) {
	// The JSON genesis document was generated from a snapshot, which itself
	// contains sufficient information to recreate the JSON genesis doc. So all
	// we need to do here is extract the snapshot.
	var b []byte
	err := cmtjson.Unmarshal(doc.AppState, &b)
	return b, err
}

// DocProvider reads the genesis document.
func DocProvider(config *tm.Config) node.GenesisDocProvider {
	// If it's a JSON file, fallback to the default
	if filepath.Ext(config.Genesis) == ".json" {
		return node.DefaultGenesisDocProviderFunc(config)
	}

	return func() (*types.GenesisDoc, error) {
		// Open the snapshot
		all, err := os.ReadFile(config.GenesisFile())
		if err != nil {
			return nil, err
		}

		return ConvertSnapshotToJson(all)
	}
}

func ConvertSnapshotToJson(snap []byte) (*types.GenesisDoc, error) {
	s, err := snapshot.Open(bytes.NewReader(snap))
	if err != nil {
		return nil, fmt.Errorf("failed to open snapshot: %w", err)
	}

	// Read the consensus section
	rd, err := s.Open(snapshot.SectionTypeConsensus)
	if err != nil {
		return nil, fmt.Errorf("failed to open consensus section: %w", err)
	}
	
	// Read the entire consensus section content
	consensusData, err := io.ReadAll(rd)
	if err != nil {
		return nil, fmt.Errorf("failed to read consensus section: %w", err)
	}
	
	// Log the consensus section content for debugging
	fmt.Printf("DEBUG: Consensus section size: %d bytes\n", len(consensusData))
	if len(consensusData) < 1000 {
		fmt.Printf("DEBUG: Consensus section content: %s\n", string(consensusData))
	} else {
		fmt.Printf("DEBUG: Consensus section content (first 1000 bytes): %s...\n", string(consensusData[:1000]))
	}
	
	p := new(cometbft.GenesisDoc)
	
	// First try to unmarshal as JSON
	err = json.Unmarshal(consensusData, p)
	if err != nil {
		fmt.Printf("DEBUG: Failed to unmarshal consensus section as JSON: %v\n", err)
		fmt.Printf("DEBUG: Falling back to binary unmarshaling\n")
		
		// Fall back to binary unmarshaling
		err = p.UnmarshalBinary(consensusData)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal consensus section (tried both JSON and binary): %w", err)
		}
	} else {
		fmt.Printf("DEBUG: Successfully unmarshaled consensus section as JSON\n")
	}

	// Convert
	jsonBytes, err := cmtjson.Marshal(snap)
	if err != nil {
		return nil, err
	}

	// Use snapshot metadata for genesis document
	genesisTime := s.Header.SystemLedger.Timestamp
	initialHeight := int64(s.Header.SystemLedger.Index) + 1
	consensusParams := (*types.ConsensusParams)(p.Params)
	doc := &types.GenesisDoc{
		GenesisTime:     genesisTime,
		ChainID:         p.ChainID,
		InitialHeight:   initialHeight,
		ConsensusParams: consensusParams,
		Validators:      make([]types.GenesisValidator, len(p.Validators)),
		AppHash:         s.Header.RootHash[:],
		AppState:        jsonBytes,
	}
	for i, v := range p.Validators {
		u := types.GenesisValidator{
			Address: v.Address,
			Power:   v.Power,
			Name:    v.Name,
		}
		switch v.Type {
		case protocol.SignatureTypeED25519:
			u.PubKey = ed25519.PubKey(v.PubKey)
		default:
			return nil, fmt.Errorf("unsupported key type %s", v.Type)
		}
		doc.Validators[i] = u
	}

	return doc, nil
}

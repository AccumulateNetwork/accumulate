// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package genesis

import (
	"bytes"
	"fmt"
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
		return nil, err
	}

	// Try to read the consensus section (may not exist or may be malformed)
	p := new(cometbft.GenesisDoc)
	rd, err := s.Open(snapshot.SectionTypeConsensus)
	if err == nil {
		// Consensus section exists, try to parse it
		if parseErr := p.UnmarshalBinaryFrom(rd); parseErr != nil {
			// Parsing failed - continue with empty GenesisDoc
			// This can happen with genesis snapshots that have partial data
			p = new(cometbft.GenesisDoc)
		}
	}
	// If consensus section doesn't exist, p remains empty

	// Convert
	jsonBytes, err := cmtjson.Marshal(snap)
	if err != nil {
		return nil, err
	}

	// Use default consensus params if not provided
	var consensusParams *types.ConsensusParams
	if p.Params != nil {
		consensusParams = (*types.ConsensusParams)(p.Params)
	} else {
		consensusParams = types.DefaultConsensusParams()
	}

	// Derive chain ID from partition URL if not provided
	chainID := p.ChainID
	if chainID == "" && s.Header.SystemLedger != nil && s.Header.SystemLedger.Url != nil {
		chainID = s.Header.SystemLedger.Url.Authority
	}
	if chainID == "" {
		chainID = "accumulate"
	}

	doc := &types.GenesisDoc{
		GenesisTime:     s.Header.SystemLedger.Timestamp,
		ChainID:         chainID,
		InitialHeight:   int64(s.Header.SystemLedger.Index) + 1,
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

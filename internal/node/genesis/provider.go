// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package genesis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/node"
	"github.com/cometbft/cometbft/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func ConvertJsonToSnapshot(doc *types.GenesisDoc) ([]byte, error) {
	// The JSON genesis document was generated from a snapshot, which itself
	// contains sufficient information to recreate the JSON genesis doc. So all
	// we need to do here is extract the snapshot.
	var b []byte
	err := json.Unmarshal(doc.AppState, &b)
	return b, err
}

// DocProvider reads the genesis document.
func DocProvider(config *config.Config) node.GenesisDocProvider {
	// If it's a JSON file, fallback to the default
	if filepath.Ext(config.Genesis) == ".json" {
		return node.DefaultGenesisDocProviderFunc(&config.Config)
	}

	return func() (*types.GenesisDoc, error) {
		// Open the snapshot
		all, err := os.ReadFile(config.GenesisFile())
		if err != nil {
			return nil, err
		}

		s, err := snapshot.Open(bytes.NewReader(all))
		if err != nil {
			return nil, err
		}

		// Read the consensus section
		rd, err := s.Open(snapshot.SectionTypeConsensus)
		if err != nil {
			return nil, err
		}
		p := new(consensusDoc)
		err = p.UnmarshalBinaryFrom(rd)
		if err != nil {
			return nil, err
		}

		// Convert
		jsonBytes, err := json.Marshal(all)
		if err != nil {
			return nil, err
		}

		doc := &types.GenesisDoc{
			GenesisTime:     s.Header.SystemLedger.Timestamp,
			ChainID:         p.ChainID,
			InitialHeight:   int64(s.Header.SystemLedger.Index) + 1,
			ConsensusParams: (*types.ConsensusParams)(p.Params),
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
}

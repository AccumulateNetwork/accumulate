// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package genesis

import (
	"bytes"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/cometbft"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ConvertSnapshotToJson converts snapshot bytes to a JSON genesis document.
func ConvertSnapshotToJson(snap []byte) (*GenesisDocJSON, error) {
	s, err := snapshot.Open(bytes.NewReader(snap))
	if err != nil {
		return nil, err
	}
	return buildGenesisDoc(s)
}

// buildGenesisDoc extracts genesis metadata from an opened snapshot.
// This only reads the header and consensus sections.
func buildGenesisDoc(s *snapshot.Reader) (*GenesisDocJSON, error) {
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

	// Don't embed the full snapshot in AppState - it wastes ~10GB of memory.
	// The ABCI app will load the snapshot directly from disk when needed.
	// We store null as a marker that the snapshot should be loaded from disk.
	jsonBytes := []byte("null")

	// Use default consensus params if not provided
	var consensusParams *ConsensusParamsJSON
	if p.Params != nil {
		consensusParams = &ConsensusParamsJSON{
			Block:     BlockParamsJSON{MaxBytes: p.Params.Block.MaxBytes, MaxGas: p.Params.Block.MaxGas},
			Evidence:  EvidenceParamsJSON{MaxAgeNumBlocks: p.Params.Evidence.MaxAgeNumBlocks, MaxAgeDuration: p.Params.Evidence.MaxAgeDuration, MaxBytes: p.Params.Evidence.MaxBytes},
			Validator: ValidatorParamsJSON{PubKeyTypes: p.Params.Validator.PubKeyTypes},
			Version:   VersionParamsJSON{App: p.Params.Version.App},
			ABCI:      ABCIParamsJSON{VoteExtensionsEnableHeight: p.Params.ABCI.VoteExtensionsEnableHeight},
		}
	} else {
		consensusParams = defaultConsensusParamsJSON()
	}

	// Derive chain ID from partition URL if not provided
	chainID := p.ChainID
	if chainID == "" && s.Header.SystemLedger != nil && s.Header.SystemLedger.Url != nil {
		chainID = s.Header.SystemLedger.Url.Authority
	}
	if chainID == "" {
		chainID = "accumulate"
	}

	doc := &GenesisDocJSON{
		GenesisTime:     s.Header.SystemLedger.Timestamp,
		ChainID:         chainID,
		InitialHeight:   int64(s.Header.SystemLedger.Index) + 1,
		ConsensusParams: consensusParams,
		Validators:      make([]GenesisValidatorJSON, len(p.Validators)),
		AppHash:         hexBytes(s.Header.RootHash[:]),
		AppState:        jsonBytes,
	}
	for i, v := range p.Validators {
		u := GenesisValidatorJSON{
			Address: hexBytes(v.Address),
			Power:   v.Power,
			Name:    v.Name,
		}
		switch v.Type {
		case protocol.SignatureTypeED25519:
			u.PubKey = pubKeyJSON(v.PubKey)
		default:
			return nil, fmt.Errorf("unsupported key type %s", v.Type)
		}
		doc.Validators[i] = u
	}

	return doc, nil
}

// defaultConsensusParamsJSON returns defaults matching CometBFT v0.38.
func defaultConsensusParamsJSON() *ConsensusParamsJSON {
	return &ConsensusParamsJSON{
		Block: BlockParamsJSON{
			MaxBytes: 22020096, // 21MB
			MaxGas:   -1,
		},
		Evidence: EvidenceParamsJSON{
			MaxAgeNumBlocks: 100000,
			MaxAgeDuration:  48 * time.Hour,
			MaxBytes:        1048576, // 1MB
		},
		Validator: ValidatorParamsJSON{
			PubKeyTypes: []string{"ed25519"},
		},
	}
}

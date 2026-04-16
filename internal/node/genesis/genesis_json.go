// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package genesis

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// GenesisDocJSON is a JSON-compatible genesis document matching CometBFT's
// types.GenesisDoc format. Used for writing and reading genesis JSON files.
type GenesisDocJSON struct {
	GenesisTime     time.Time             `json:"genesis_time"`
	ChainID         string                `json:"chain_id"`
	InitialHeight   int64                 `json:"initial_height"`
	ConsensusParams *ConsensusParamsJSON  `json:"consensus_params,omitempty"`
	Validators      []GenesisValidatorJSON `json:"validators,omitempty"`
	AppHash         hexBytes              `json:"app_hash"`
	AppState        json.RawMessage       `json:"app_state,omitempty"`
}

// GenesisValidatorJSON mirrors CometBFT's types.GenesisValidator JSON format.
type GenesisValidatorJSON struct {
	Address hexBytes   `json:"address"`
	PubKey  pubKeyJSON `json:"pub_key"`
	Power   int64      `json:"power"`
	Name    string     `json:"name"`
}

// ConsensusParamsJSON mirrors CometBFT's types.ConsensusParams JSON format.
type ConsensusParamsJSON struct {
	Block     BlockParamsJSON     `json:"block"`
	Evidence  EvidenceParamsJSON  `json:"evidence"`
	Validator ValidatorParamsJSON `json:"validator"`
	Version   VersionParamsJSON   `json:"version"`
	ABCI      ABCIParamsJSON      `json:"abci"`
}

type BlockParamsJSON struct {
	MaxBytes int64 `json:"max_bytes"`
	MaxGas   int64 `json:"max_gas"`
}

type EvidenceParamsJSON struct {
	MaxAgeNumBlocks int64         `json:"max_age_num_blocks"`
	MaxAgeDuration  time.Duration `json:"max_age_duration"`
	MaxBytes        int64         `json:"max_bytes"`
}

type ValidatorParamsJSON struct {
	PubKeyTypes []string `json:"pub_key_types"`
}

type VersionParamsJSON struct {
	App uint64 `json:"app"`
}

type ABCIParamsJSON struct {
	VoteExtensionsEnableHeight int64 `json:"vote_extensions_enable_height"`
}

// hexBytes is a []byte that marshals to/from uppercase hex strings in JSON,
// matching CometBFT's libs/bytes.HexBytes.
type hexBytes []byte

func (b hexBytes) MarshalJSON() ([]byte, error) {
	s := strings.ToUpper(hex.EncodeToString(b))
	return json.Marshal(s)
}

func (b *hexBytes) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	decoded, err := hex.DecodeString(s)
	if err != nil {
		return err
	}
	*b = decoded
	return nil
}

// pubKeyJSON marshals ed25519 public keys in CometBFT's amino JSON format:
// {"type":"tendermint/PubKeyEd25519","value":"<base64>"}
type pubKeyJSON []byte

func (k pubKeyJSON) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}{
		Type:  "tendermint/PubKeyEd25519",
		Value: base64.StdEncoding.EncodeToString(k),
	})
}

func (k *pubKeyJSON) UnmarshalJSON(data []byte) error {
	var v struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if v.Type != "tendermint/PubKeyEd25519" {
		return fmt.Errorf("unsupported key type %q", v.Type)
	}
	decoded, err := base64.StdEncoding.DecodeString(v.Value)
	if err != nil {
		return err
	}
	*k = decoded
	return nil
}

// GenesisDocFromFile reads a genesis JSON file and returns a GenesisDocJSON.
func GenesisDocFromFile(path string) (*GenesisDocJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read genesis file: %w", err)
	}
	doc := new(GenesisDocJSON)
	if err := json.Unmarshal(data, doc); err != nil {
		return nil, fmt.Errorf("parse genesis file: %w", err)
	}
	return doc, nil
}

// SaveAs writes the genesis document to the given file path.
func (doc *GenesisDocJSON) SaveAs(path string) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal genesis document: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

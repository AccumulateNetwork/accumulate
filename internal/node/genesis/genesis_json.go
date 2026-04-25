// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package genesis

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// MaxChainIDLen is the maximum allowed length of the genesis chain_id,
// matching CometBFT's types.MaxChainIDLen.
const MaxChainIDLen = 50

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

// GenesisDocFromFile reads a genesis JSON file and returns a validated
// GenesisDocJSON. ValidateAndComplete is invoked automatically, matching
// CometBFT's GenesisDocFromJSON → ValidateAndComplete chain.
func GenesisDocFromFile(path string) (*GenesisDocJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read genesis file: %w", err)
	}
	doc := new(GenesisDocJSON)
	if err := json.Unmarshal(data, doc); err != nil {
		return nil, fmt.Errorf("parse genesis file: %w", err)
	}
	if err := doc.ValidateAndComplete(); err != nil {
		return nil, fmt.Errorf("invalid genesis file %s: %w", path, err)
	}
	return doc, nil
}

// Address returns the CometBFT-style address for an ed25519 public key:
// the first 20 bytes of SHA256(pubkey). Matches CometBFT's
// crypto.ed25519.PubKey.Address.
func (k pubKeyJSON) Address() []byte {
	h := sha256.Sum256(k)
	return h[:20]
}

// ValidateAndComplete validates the genesis document and fills in defaults
// for unset optional fields, matching CometBFT v0.38's
// types.GenesisDoc.ValidateAndComplete:
//   - chain_id must be non-empty and at most MaxChainIDLen.
//   - initial_height must be non-negative; 0 is normalized to 1.
//   - every validator must have non-zero power; an explicit address must
//     match the public key's derived address (or be empty, in which case
//     it is filled in).
//   - genesis_time defaults to time.Now() if zero.
//   - consensus_params is filled with defaults if nil; otherwise
//     validated.
func (doc *GenesisDocJSON) ValidateAndComplete() error {
	if doc.ChainID == "" {
		return errors.New("genesis doc must include non-empty chain_id")
	}
	if len(doc.ChainID) > MaxChainIDLen {
		return fmt.Errorf("chain_id in genesis doc is too long (max %d, got %d)", MaxChainIDLen, len(doc.ChainID))
	}
	if doc.InitialHeight < 0 {
		return fmt.Errorf("initial_height cannot be negative (got %d)", doc.InitialHeight)
	}
	if doc.InitialHeight == 0 {
		doc.InitialHeight = 1
	}

	if doc.ConsensusParams == nil {
		doc.ConsensusParams = defaultConsensusParamsJSON()
	} else if err := doc.ConsensusParams.validateBasic(); err != nil {
		return err
	}

	for i, v := range doc.Validators {
		if v.Power == 0 {
			return fmt.Errorf("validator %d (%s): voting power cannot be zero", i, v.Name)
		}
		if len(v.PubKey) != 32 {
			return fmt.Errorf("validator %d (%s): pub_key must be 32 bytes (got %d)", i, v.Name, len(v.PubKey))
		}
		derived := v.PubKey.Address()
		if len(v.Address) > 0 && !bytes.Equal(v.Address, derived) {
			return fmt.Errorf("validator %d (%s): address %X does not match pub_key-derived address %X",
				i, v.Name, []byte(v.Address), derived)
		}
		if len(v.Address) == 0 {
			doc.Validators[i].Address = derived
		}
	}

	if doc.GenesisTime.IsZero() {
		doc.GenesisTime = time.Now().UTC()
	}

	return nil
}

// validateBasic mirrors CometBFT v0.38's ConsensusParams.ValidateBasic,
// scoped to fields that have meaningful constraints. Negative or zero
// limits where positive values are required are rejected.
func (p *ConsensusParamsJSON) validateBasic() error {
	if p.Block.MaxBytes == 0 {
		return errors.New("consensus_params.block.max_bytes cannot be 0")
	}
	if p.Block.MaxBytes < -1 {
		return fmt.Errorf("consensus_params.block.max_bytes must be -1 or > 0 (got %d)", p.Block.MaxBytes)
	}
	if p.Block.MaxGas < -1 {
		return fmt.Errorf("consensus_params.block.max_gas must be -1 or >= 0 (got %d)", p.Block.MaxGas)
	}
	if p.Evidence.MaxAgeNumBlocks <= 0 {
		return fmt.Errorf("consensus_params.evidence.max_age_num_blocks must be > 0 (got %d)", p.Evidence.MaxAgeNumBlocks)
	}
	if p.Evidence.MaxAgeDuration <= 0 {
		return fmt.Errorf("consensus_params.evidence.max_age_duration must be > 0 (got %s)", p.Evidence.MaxAgeDuration)
	}
	if p.Evidence.MaxBytes < 0 {
		return fmt.Errorf("consensus_params.evidence.max_bytes must be >= 0 (got %d)", p.Evidence.MaxBytes)
	}
	if p.Evidence.MaxBytes > p.Block.MaxBytes {
		return fmt.Errorf("consensus_params.evidence.max_bytes (%d) is greater than consensus_params.block.max_bytes (%d)",
			p.Evidence.MaxBytes, p.Block.MaxBytes)
	}
	if len(p.Validator.PubKeyTypes) == 0 {
		return errors.New("consensus_params.validator.pub_key_types must contain at least one type")
	}
	for i, t := range p.Validator.PubKeyTypes {
		if t == "" {
			return fmt.Errorf("consensus_params.validator.pub_key_types[%d] is empty", i)
		}
	}
	if p.ABCI.VoteExtensionsEnableHeight < 0 {
		return fmt.Errorf("consensus_params.abci.vote_extensions_enable_height cannot be negative (got %d)",
			p.ABCI.VoteExtensionsEnableHeight)
	}
	return nil
}

// SaveAs writes the genesis document to the given file path.
func (doc *GenesisDocJSON) SaveAs(path string) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal genesis document: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

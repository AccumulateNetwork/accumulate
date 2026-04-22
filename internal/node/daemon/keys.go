// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

// Local key types that replicate CometBFT's privval.FilePV, privval.FilePVKey,
// privval.FilePVLastSignState, and p2p.NodeKey with identical JSON
// serialization. This allows reading and writing the same
// priv_validator_key.json, priv_validator_state.json, and node_key.json files
// without importing CometBFT.

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// CometBFT address size: first 20 bytes of SHA256(pubkey).
const addressSize = 20

// pubKeyAddress returns the CometBFT-style address for an ed25519 public key:
// the first 20 bytes of SHA256(pubkey).
func pubKeyAddress(pub ed25519.PublicKey) []byte {
	h := sha256.Sum256(pub)
	return h[:addressSize]
}

// PubKeyToID returns the hex-encoded address for the given public key,
// matching CometBFT's p2p.PubKeyToID.
func PubKeyToID(pub ed25519.PublicKey) string {
	return hex.EncodeToString(pubKeyAddress(pub))
}

// IDAddressString returns id@host:port, stripping any leading scheme from
// protocolHostPort. Matches CometBFT's p2p.IDAddressString.
func IDAddressString(id string, protocolHostPort string) string {
	// Strip scheme (e.g. "tcp://")
	if idx := strings.Index(protocolHostPort, "://"); idx >= 0 {
		protocolHostPort = protocolHostPort[idx+3:]
	}
	return id + "@" + protocolHostPort
}

// cometKey is a JSON-compatible representation of a CometBFT crypto key.
// CometBFT serializes interface keys as {"type":"tendermint/...","value":"base64..."}.
type cometKey struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

func marshalPrivKey(key ed25519.PrivateKey) cometKey {
	return cometKey{
		Type:  "tendermint/PrivKeyEd25519",
		Value: base64.StdEncoding.EncodeToString(key),
	}
}

func marshalPubKey(key ed25519.PublicKey) cometKey {
	return cometKey{
		Type:  "tendermint/PubKeyEd25519",
		Value: base64.StdEncoding.EncodeToString(key),
	}
}

func unmarshalPrivKey(ck cometKey) (ed25519.PrivateKey, error) {
	if ck.Type != "tendermint/PrivKeyEd25519" {
		return nil, fmt.Errorf("unsupported key type: %s", ck.Type)
	}
	b, err := base64.StdEncoding.DecodeString(ck.Value)
	if err != nil {
		return nil, err
	}
	return ed25519.PrivateKey(b), nil
}

// FilePVKey stores the immutable part of a private validator.
// JSON format matches CometBFT's privval.FilePVKey exactly.
type FilePVKey struct {
	Address  []byte             `json:"-"` // derived, not in JSON input
	PubKey   ed25519.PublicKey  `json:"-"` // derived from PrivKey
	PrivKey  ed25519.PrivateKey `json:"-"`
	filePath string
}

// filePVKeyJSON is the JSON representation matching CometBFT's format.
type filePVKeyJSON struct {
	Address string   `json:"address"`
	PubKey  cometKey `json:"pub_key"`
	PrivKey cometKey `json:"priv_key"`
}

func (k FilePVKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(filePVKeyJSON{
		Address: strings.ToUpper(hex.EncodeToString(k.Address)),
		PubKey:  marshalPubKey(k.PubKey),
		PrivKey: marshalPrivKey(k.PrivKey),
	})
}

func (k *FilePVKey) UnmarshalJSON(data []byte) error {
	var j filePVKeyJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	var err error
	k.PrivKey, err = unmarshalPrivKey(j.PrivKey)
	if err != nil {
		return fmt.Errorf("priv_key: %w", err)
	}
	// Derive pub key and address from private key for consistency
	k.PubKey = k.PrivKey.Public().(ed25519.PublicKey)
	k.Address = pubKeyAddress(k.PubKey)
	return nil
}

func (k FilePVKey) Save() {
	if k.filePath == "" {
		panic("cannot save PrivValidator key: filePath not set")
	}
	data, err := json.MarshalIndent(k, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(k.filePath, data, 0600); err != nil {
		panic(err)
	}
}

// FilePVLastSignState stores the mutable part of a private validator.
// JSON format matches CometBFT's privval.FilePVLastSignState.
type FilePVLastSignState struct {
	Height    int64  `json:"height,string"`
	Round     int32  `json:"round"`
	Step      int8   `json:"step"`
	Signature []byte `json:"signature,omitempty"`
	SignBytes []byte `json:"signbytes,omitempty"`
	filePath  string
}

func (lss *FilePVLastSignState) Save() {
	if lss.filePath == "" {
		panic("cannot save FilePVLastSignState: filePath not set")
	}
	data, err := json.MarshalIndent(lss, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(lss.filePath, data, 0600); err != nil {
		panic(err)
	}
}

// FilePV implements PrivValidator using data persisted to disk.
// Matches CometBFT's privval.FilePV.
type FilePV struct {
	Key           FilePVKey
	LastSignState FilePVLastSignState
}

// NewFilePV creates a new FilePV with the given private key and file paths.
func NewFilePV(privKey ed25519.PrivateKey, keyFilePath, stateFilePath string) *FilePV {
	pubKey := privKey.Public().(ed25519.PublicKey)
	return &FilePV{
		Key: FilePVKey{
			Address:  pubKeyAddress(pubKey),
			PubKey:   pubKey,
			PrivKey:  privKey,
			filePath: keyFilePath,
		},
		LastSignState: FilePVLastSignState{
			Step:     0,
			filePath: stateFilePath,
		},
	}
}

func (pv *FilePV) Save() {
	pv.Key.Save()
	pv.LastSignState.Save()
}

// LoadFilePV loads a FilePV from the given file paths.
func LoadFilePV(keyFilePath, stateFilePath string) (*FilePV, error) {
	keyData, err := os.ReadFile(keyFilePath)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}
	var key FilePVKey
	if err := json.Unmarshal(keyData, &key); err != nil {
		return nil, fmt.Errorf("unmarshal key file %s: %w", keyFilePath, err)
	}
	key.filePath = keyFilePath

	var state FilePVLastSignState
	stateData, err := os.ReadFile(stateFilePath)
	if err != nil {
		return nil, fmt.Errorf("read state file: %w", err)
	}
	if err := json.Unmarshal(stateData, &state); err != nil {
		return nil, fmt.Errorf("unmarshal state file %s: %w", stateFilePath, err)
	}
	state.filePath = stateFilePath

	return &FilePV{
		Key:           key,
		LastSignState: state,
	}, nil
}

// NodeKey is the persistent peer key. Matches CometBFT's p2p.NodeKey.
type NodeKey struct {
	PrivKey ed25519.PrivateKey `json:"-"`
}

// nodeKeyJSON matches CometBFT's JSON format for NodeKey.
type nodeKeyJSON struct {
	PrivKey cometKey `json:"priv_key"`
}

func (nk NodeKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(nodeKeyJSON{
		PrivKey: marshalPrivKey(nk.PrivKey),
	})
}

func (nk *NodeKey) UnmarshalJSON(data []byte) error {
	var j nodeKeyJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	var err error
	nk.PrivKey, err = unmarshalPrivKey(j.PrivKey)
	if err != nil {
		return err
	}
	return nil
}

// ID returns the hex-encoded address of the node's public key.
func (nk *NodeKey) ID() string {
	return PubKeyToID(nk.PrivKey.Public().(ed25519.PublicKey))
}

// PubKey returns the public key of the node key.
func (nk *NodeKey) PubKey() ed25519.PublicKey {
	return nk.PrivKey.Public().(ed25519.PublicKey)
}

// SaveAs saves the node key to the given file path.
func (nk *NodeKey) SaveAs(filePath string) error {
	data, err := json.Marshal(nk)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0600)
}

// LoadNodeKey loads a NodeKey from the given file path.
func LoadNodeKey(filePath string) (*NodeKey, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	nk := new(NodeKey)
	if err := json.Unmarshal(data, nk); err != nil {
		return nil, err
	}
	return nk, nil
}

// fileExists is a simple replacement for tmos.FileExists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

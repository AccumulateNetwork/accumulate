// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
)

// The key conversion (network spec, "Converting a mainnet node"): a node
// initialized for CometBFT holds its node and validator keys in CometBFT's
// JSON key files (node_key.json, priv_validator_key.json). The node reads
// neither; this tool reads them and writes each key inline, as a raw private
// key — the form `accumulated init network` writes.

// cometKeyFile is the JSON layout of node_key.json and priv_validator_key.json.
type cometKeyFile struct {
	PrivKey struct {
		Type  string `json:"type"`
		Value []byte `json:"value"` // base64
	} `json:"priv_key"`
}

// readCometKey reads a CometBFT key file and returns the key inline.
func readCometKey(file string) (*run.RawPrivateKey, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var kf cometKeyFile
	err = json.Unmarshal(b, &kf)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if kf.PrivKey.Type != "tendermint/PrivKeyEd25519" {
		return nil, fmt.Errorf("%s: key type %q is not supported", file, kf.PrivKey.Type)
	}
	if len(kf.PrivKey.Value) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%s: invalid ed25519 key size %d", file, len(kf.PrivKey.Value))
	}
	sk := ed25519.PrivateKey(kf.PrivKey.Value)
	return &run.RawPrivateKey{Address: address.FromED25519PrivateKey(sk).String()}, nil
}

// The key types a configuration written by main's `accumulated migrate` (or
// by an earlier migrate-legacy) names, and their JSON/TOML discriminators.
const (
	cometPrivValFile = "cometPrivValFile"
	cometNodeKeyFile = "cometNodeKeyFile"
)

func isCometKeyType(s string) bool {
	return strings.EqualFold(s, cometPrivValFile) || strings.EqualFold(s, cometNodeKeyFile)
}

// convertKeyRefs rewrites, in a node configuration decoded to a generic
// document, every key that names a CometBFT key file — p2p.key and each
// configuration's validator-key — as the key itself, inline. A core validator
// or follower with no p2p.key gets the node key main's node defaulted to,
// {dnn|bvnn}/config/node_key.json, if that file exists. Relative paths are
// resolved against dir, the configuration file's directory, as the node
// resolved them. It reports whether anything changed.
func convertKeyRefs(doc map[string]any, dir string) (bool, error) {
	var changed bool
	convert := func(m map[string]any, field string) error {
		ref, ok := m[field].(map[string]any)
		if !ok {
			return nil
		}
		typ, _ := ref["type"].(string)
		if !isCometKeyType(typ) {
			return nil
		}
		path, _ := ref["path"].(string)
		if path == "" {
			return fmt.Errorf("%s: a %s key names no path", field, typ)
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		key, err := readCometKey(path)
		if err != nil {
			return err
		}
		m[field] = map[string]any{"type": "raw", "address": key.Address}
		changed = true
		return nil
	}

	p2p, ok := doc["p2p"].(map[string]any)
	if !ok {
		p2p = map[string]any{}
	}
	err := convert(p2p, "key")
	if err != nil {
		return false, fmt.Errorf("p2p.%w", err)
	}

	var configs []map[string]any
	switch v := doc["configurations"].(type) {
	case []map[string]any:
		configs = v
	case []any:
		for _, c := range v {
			if c, ok := c.(map[string]any); ok {
				configs = append(configs, c)
			}
		}
	}
	for i, c := range configs {
		err = convert(c, "validator-key")
		if err != nil {
			return false, fmt.Errorf("configurations[%d].%w", i, err)
		}

		// Main's core validator and follower defaulted the node key to the
		// partition's CometBFT node key file.
		typ, _ := c["type"].(string)
		if _, has := p2p["key"]; has || !(strings.EqualFold(typ, "coreValidator") || strings.EqualFold(typ, "follower")) {
			continue
		}
		part := "dnn"
		if mode, _ := c["mode"].(string); strings.EqualFold(mode, "bvn") {
			part = "bvnn"
		}
		file := filepath.Join(dir, part, "config", "node_key.json")
		if _, err := os.Stat(file); err != nil {
			fmt.Fprintf(os.Stderr, "No p2p.key and no %s: the converted configuration names no node key; a validator will refuse to start, a follower will generate one (#4490)\n", file)
			continue
		}
		key, err := readCometKey(file)
		if err != nil {
			return false, err
		}
		p2p["key"] = map[string]any{"type": "raw", "address": key.Address}
		doc["p2p"] = p2p
		changed = true
	}
	return changed, nil
}

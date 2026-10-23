// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package anchoring holds the types of Bitcoin anchoring (docs/spec/anchoring.md,
// #4562): the evidence package a stranger verifies with Bitcoin alone, and —
// as the parts are built — the OP_RETURN payload, the address list and the
// coordination records.
package anchoring

import (
	"encoding/hex"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Package is the evidence package (anchoring.md §2.5): one chain entry of one
// account, its receipt to a Directory root, and the Bitcoin anchor that
// committed that root. Anchor is nil until an anchor covers the root.
type Package struct {
	Network   string          `json:"network"`
	Account   *url.URL        `json:"account"`
	Entry     Entry           `json:"entry"`
	Receipt   *merkle.Receipt `json:"receipt"`
	Directory Directory       `json:"directory"`
	Anchor    *Anchor         `json:"anchor,omitempty"`
}

// Entry names the chain entry the package proves.
type Entry struct {
	Chain string   `json:"chain"`
	Index uint64   `json:"index"`
	Hash  HexBytes `json:"hash"`
}

// Directory is the root the receipt terminates at.
type Directory struct {
	Height uint64   `json:"height"`
	Root   HexBytes `json:"root"`
}

// Anchor is the Bitcoin transaction that committed the Directory root, with
// its coordination record (B2, B4).
type Anchor struct {
	TxID        string `json:"txid"`
	Block       string `json:"block"`
	BlockHeight uint64 `json:"blockHeight"`
}

// HexBytes is a byte string that encodes as a hex string in JSON (anchoring.md
// §2.5: every byte string in the package is hex).
type HexBytes []byte

func (h HexBytes) MarshalText() ([]byte, error) {
	return []byte(hex.EncodeToString(h)), nil
}

func (h *HexBytes) UnmarshalText(text []byte) error {
	b, err := hex.DecodeString(string(text))
	if err != nil {
		return fmt.Errorf("invalid hex: %w", err)
	}
	*h = b
	return nil
}

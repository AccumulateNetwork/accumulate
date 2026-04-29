// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package bootpersist persists bootstrap-v3 state across restarts.
//
// The v3 artifact records what cannot be reconstructed from the
// local DB alone:
//
//   - Node-state machine snapshot (BOOTING / ACTIVE / COMPLETE plus
//     anchor + sinceBlock).
//   - Phase progress (spine pull done, enumerate done, enumerate
//     resume cursor) so a crash mid-bootstrap doesn't redo work.
//   - Observed-anchor map so the tracker resumes with prior
//     candidates without re-querying the network from scratch.
//
// The local DB itself persists naturally — leaves inserted by
// enumerate stay across restarts. This artifact is the
// reconstruction-impossible delta.
//
// Format guarantees:
//   - FormatMajor mismatch on Load is a hard error (incompatible).
//   - FormatMinor is forward-compatible: loaders accept any minor
//     <= their own and ignore unknown fields (encoding/json default).
//   - Atomic save: write to a temp file in the same directory, then
//     rename, so a crash mid-write doesn't leave a half-written
//     artifact.
package bootpersist

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// FormatMajor is the artifact's major version. Loaders reject any
// major version they don't recognize. Bumped to 3 with the
// bootstrap-v3 sync-protocol schema (no pinned hash, partition-
// scoped). Pre-3 artifacts are not migrated in place.
const FormatMajor = 3

// FormatMinor is the artifact's minor version. Loaders tolerate any
// minor version <= their own and ignore unknown fields.
const FormatMinor = 0

// FileName is the on-disk name of the artifact within the data dir.
const FileName = "bootstrap-state.json"

// Artifact is the top-level persisted object for one partition's
// bootstrap progress. Multi-partition nodes hold one artifact per
// data dir / partition.
type Artifact struct {
	FormatMajor uint32 `json:"formatMajor"`
	FormatMinor uint32 `json:"formatMinor"`

	// Network identifies which network this bootstrap belongs to —
	// "mainnet", "testnet", "devnet", or a custom name. Caller-set;
	// the package does not enforce semantics.
	Network string `json:"network,omitempty"`

	// Partition is the partition this artifact tracks — "Directory"
	// for the DN, "<bvn-name>" for a BVN.
	Partition string `json:"partition"`

	// State is the persisted node-state machine snapshot.
	State StateRecord `json:"state"`

	// Phases tracks completion of each bootstrap phase so a restart
	// resumes without redoing work.
	Phases Phases `json:"phases"`

	// ObservedAnchors lets the tracker resume with prior anchor
	// candidates after a restart, avoiding an immediate re-query of
	// the AnchorSource. Each entry is one (block, anchor) pair the
	// tracker has seen and considered valid.
	ObservedAnchors []ObservedAnchor `json:"observedAnchors,omitempty"`
}

// StateRecord captures node state and transition history.
type StateRecord struct {
	// Current is one of "BOOTING", "ACTIVE", "COMPLETE".
	Current string `json:"current"`

	// SinceBlock is the block height at which the current state
	// became true (set on transition).
	SinceBlock uint64 `json:"sinceBlock,omitempty"`

	// VerifiedAnchor is the BPT root that validated the
	// ACTIVE/COMPLETE claim. Empty for BOOTING.
	VerifiedAnchor [32]byte `json:"verifiedAnchor,omitzero"`

	// HistoryDepth is the oldest block fully retained, set at the
	// ACTIVE → COMPLETE transition. Zero means unlimited (full
	// history retained).
	HistoryDepth uint64 `json:"historyDepth,omitempty"`

	// EnteredBooting / EnteredActive / EnteredComplete record when
	// each transition occurred. Zero for never-reached states.
	EnteredBooting  time.Time `json:"enteredBooting,omitzero"`
	EnteredActive   time.Time `json:"enteredActive,omitzero"`
	EnteredComplete time.Time `json:"enteredComplete,omitzero"`
}

// Phases tracks per-phase completion / resume cursors.
type Phases struct {
	// SpinePullDone is true once the DN-side spine pull (#3985 phase 1)
	// has committed for this artifact. Always false on BVN artifacts.
	SpinePullDone bool `json:"spinePullDone,omitempty"`

	// EnumerateDone is true once the BPT enumeration scan reached
	// page.Done == true. After this, only steady-state remains.
	EnumerateDone bool `json:"enumerateDone,omitempty"`

	// EnumerateNextStart is the resume cursor for the BPT enumeration
	// scan. Zero means "start from the top" (FullScanStart). Updated
	// after every successful page commit so a restart picks up where
	// the scan left off.
	EnumerateNextStart [32]byte `json:"enumerateNextStart,omitzero"`
}

// ObservedAnchor is one (block, anchor) pair the tracker has seen.
type ObservedAnchor struct {
	Block  uint64   `json:"block"`
	Anchor [32]byte `json:"anchor"`
}

// ErrFormatMajor is returned when the persisted format major doesn't
// match this binary's. Indicates an incompatible artifact.
var ErrFormatMajor = errors.New("bootpersist: incompatible format major version")

// Load reads and validates the artifact at `dir`. Returns
// os.ErrNotExist if the file is absent. Returns ErrFormatMajor for
// incompatible major versions.
func Load(dir string) (*Artifact, error) {
	path := filepath.Join(dir, FileName)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	a, err := decode(f)
	if err != nil {
		return nil, err
	}
	if a.FormatMajor != FormatMajor {
		return nil, fmt.Errorf("%w: persisted=%d, this binary=%d", ErrFormatMajor, a.FormatMajor, FormatMajor)
	}
	return a, nil
}

// Save writes the artifact to `dir` atomically (write to temp +
// rename). The file is overwritten if it exists. FormatMajor and
// FormatMinor are forced to this binary's values; any caller-set
// values in those fields are ignored.
func Save(dir string, a *Artifact) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	a.FormatMajor = FormatMajor
	a.FormatMinor = FormatMinor

	path := filepath.Join(dir, FileName)
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", tmp, err)
	}
	if err := encode(f, a); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

func decode(r io.Reader) (*Artifact, error) {
	dec := json.NewDecoder(r)
	a := new(Artifact)
	if err := dec.Decode(a); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return a, nil
}

func encode(w io.Writer, a *Artifact) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(a); err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	return nil
}

// HexKey is a convenience for tests / logging that want to render a
// [32]byte without importing encoding/hex.
func HexKey(k [32]byte) string { return hex.EncodeToString(k[:]) }

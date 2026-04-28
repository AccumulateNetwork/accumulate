// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package bootpersist persists bootstrap state across restarts (issue
// #3965, parent #3953).
//
// The persisted artifact is a versioned forward-compatible envelope
// containing:
//
//   - Pinned genesis snapshot hash (from the binary at bootstrap time).
//   - Bootstrap pin block H (height + anchor BPT root captured at H).
//   - Node state and transition history (BOOTING / ACTIVE / COMPLETE).
//   - Validated graph (memoization records + per-account back-walks).
//   - Hydration cursors (BPT enumeration progress, traffic-listener high
//     water mark).
//   - Rolling-window of retained recent blocks/transactions.
//
// On restart, accumulated run loads the artifact via Load and resumes
// from the recorded state. A pinned-genesis-hash mismatch (binary vs.
// artifact) aborts startup unless an explicit migration is run.
package bootpersist

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// FormatMajor is the artifact's major version. Loaders reject any major
// version they don't recognize.
const FormatMajor = 1

// FormatMinor is the artifact's minor version. Loaders tolerate any
// minor version <= their own and ignore unknown fields.
const FormatMinor = 0

// FileName is the on-disk name of the artifact within the data dir.
const FileName = "bootstrap-state.json"

// Artifact is the top-level persisted object.
type Artifact struct {
	FormatMajor uint32 `json:"formatMajor"`
	FormatMinor uint32 `json:"formatMinor"`

	// PinnedGenesisHash is the hash of the genesis snapshot the binary
	// was built against. Mismatch on restart aborts startup unless an
	// explicit migration is run.
	PinnedGenesisHash [32]byte `json:"pinnedGenesisHash"`

	// Network identifies which network the node bootstrapped against.
	Network string `json:"network"`

	// PinBlock is the block H pinned at bootstrap moment (confirmed
	// depth, not live tip).
	PinBlock PinBlock `json:"pinBlock"`

	// State is the current node state and its transition history.
	State StateRecord `json:"state"`

	// Proof is the validated back-walk graph (proof of derivation).
	// Optional; populated by #3960 once back-walks complete.
	Proof *ProofRecord `json:"proof,omitempty"`

	// Cursors track hydration progress so a restart resumes without
	// redoing work.
	Cursors Cursors `json:"cursors"`
}

// PinBlock identifies the block at which initial state was captured.
type PinBlock struct {
	Partition       string   `json:"partition"`
	MajorBlockIndex uint64   `json:"majorBlockIndex"`
	MinorBlockIndex uint64   `json:"minorBlockIndex"`
	StateTreeAnchor [32]byte `json:"stateTreeAnchor"`
}

// StateRecord captures node state and its transition history.
type StateRecord struct {
	// Current is one of "BOOTING", "ACTIVE", "COMPLETE".
	Current string `json:"current"`

	// EnteredBooting / EnteredActive / EnteredComplete record when each
	// transition occurred. Zero for never-reached states.
	EnteredBooting  time.Time `json:"enteredBooting,omitzero"`
	EnteredActive   time.Time `json:"enteredActive,omitzero"`
	EnteredComplete time.Time `json:"enteredComplete,omitzero"`

	// BptRootMatched is the BPT root that validated the BOOTING → ACTIVE
	// transition. Zero before ACTIVE.
	BptRootMatched [32]byte `json:"bptRootMatched,omitzero"`

	// HistoryDepth is the oldest block fully retained, set at the
	// ACTIVE → COMPLETE transition. Zero means unlimited.
	HistoryDepth uint64 `json:"historyDepth,omitempty"`
}

// ProofRecord is the persisted form of the back-walk proof of derivation.
// Concrete shape is owned by issue #3960; this is a placeholder envelope.
type ProofRecord struct {
	// Memoizations is the list of cached (account, block_time) → resolved
	// keypage entries. Schema details deferred.
	Memoizations json.RawMessage `json:"memoizations,omitempty"`

	// AccountWalks is the list of per-account validated back-walks.
	// Schema details deferred.
	AccountWalks json.RawMessage `json:"accountWalks,omitempty"`

	// GenesisTerminations is the list of leaves whose chains bottomed
	// out at the genesis snapshot. Schema details deferred.
	GenesisTerminations json.RawMessage `json:"genesisTerminations,omitempty"`
}

// Cursors track hydration progress so a restart resumes without
// redoing work.
type Cursors struct {
	// BptPageNext is the next start hash for paginated BPT enumeration
	// (see #3969). Zero means enumeration hasn't started; "done" is
	// signaled by BptPageDone.
	BptPageNext [32]byte `json:"bptPageNext,omitzero"`
	BptPageDone bool     `json:"bptPageDone"`

	// TrafficHighWater is the highest minor-block index the live traffic
	// listener has scanned for account references.
	TrafficHighWater uint64 `json:"trafficHighWater"`

	// HistoryBackfillTarget is the oldest block the backfill (#3967) is
	// trying to reach. Zero means unlimited.
	HistoryBackfillTarget uint64 `json:"historyBackfillTarget,omitempty"`

	// HistoryBackfillReached is the oldest block fully retrieved so far.
	HistoryBackfillReached uint64 `json:"historyBackfillReached,omitempty"`
}

// ErrPinMismatch is returned when the persisted pinned genesis hash
// doesn't match what the binary expects on startup.
var ErrPinMismatch = errors.New("bootpersist: pinned genesis snapshot hash mismatch — explicit migration required")

// ErrFormatMajor is returned when the persisted format major doesn't
// match this binary's. Indicates an incompatible artifact.
var ErrFormatMajor = errors.New("bootpersist: incompatible format major version")

// Load reads the artifact from `dir` and verifies it against the
// expected pinned genesis hash. Returns os.IsNotExist if the file is
// absent. Returns ErrPinMismatch if the persisted hash differs from
// expectedPinnedHash. Returns ErrFormatMajor for incompatible major
// versions.
func Load(dir string, expectedPinnedHash [32]byte) (*Artifact, error) {
	a, err := Peek(dir)
	if err != nil {
		return nil, err
	}
	if a.PinnedGenesisHash != expectedPinnedHash {
		return nil, ErrPinMismatch
	}
	return a, nil
}

// Peek reads the artifact from `dir` and validates the format-major
// version, but does not enforce a pinned-hash check. Callers that
// need to read the artifact's Network field before resolving the
// expected pin (e.g., during accumulated run startup) use this and
// then enforce the pin themselves.
func Peek(dir string) (*Artifact, error) {
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

// Save writes the artifact to `dir`, atomically (write to temp + rename).
// The file is overwritten if it exists.
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

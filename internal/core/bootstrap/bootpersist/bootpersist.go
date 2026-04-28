// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package bootpersist persists v2 bootstrap state across restarts.
//
// The v2 artifact is much smaller than v1's: there is no genesis
// hash, no per-account back-walk record, no per-partition anchor
// list. The proof shape is "verified header anchor + local BPT
// matching it," so the persisted form just records that pair plus
// enough cursor state to resume mid-bootstrap.
//
// Format guarantees:
//   - FormatMajor mismatch on Load is a hard error (incompatible).
//   - FormatMinor is forward-compatible: loaders accept any minor
//     <= their own and ignore unknown fields (encoding/json default).
//   - Atomic save: write to a temp file in the same directory, then
//     rename, so a crash mid-write doesn't leave a half-written
//     artifact.
//
// Pin enforcement is enforced separately by the caller via Peek (read
// without verifying) followed by an explicit pinned-validator-set
// match. The package itself does not embed pin policy.
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

// FormatMajor is the artifact's major version. Loaders reject any
// major version they don't recognize.
const FormatMajor = 1

// FormatMinor is the artifact's minor version. Loaders tolerate any
// minor version <= their own and ignore unknown fields.
const FormatMinor = 0

// FileName is the on-disk name of the artifact within the data dir.
const FileName = "bootstrap-state-v2.json"

// Artifact is the top-level persisted object.
type Artifact struct {
	FormatMajor uint32 `json:"formatMajor"`
	FormatMinor uint32 `json:"formatMinor"`

	// Network identifies which network the launcher bootstrapped
	// against. Used by callers to resolve the binary's pinned
	// validator-set hash for that network at startup.
	Network string `json:"network"`

	// Partition the launcher is participating in (Directory or a
	// BVN name).
	Partition string `json:"partition"`

	// PinnedValidatorSetHash is the hash of the validator set the
	// launcher was bootstrapped against. The walk's first header
	// must be verifiable against the validator set whose hash
	// matches this.
	PinnedValidatorSetHash [32]byte `json:"pinnedValidatorSetHash"`

	// PinnedHeight is the height the launcher started its walk at.
	PinnedHeight uint64 `json:"pinnedHeight"`

	// VerifiedAnchor is the StateTreeRoot from the terminal header
	// the trust phase verified. The local BPT root must equal this
	// for the launcher to be in (or above) ACTIVE.
	VerifiedAnchor [32]byte `json:"verifiedAnchor,omitzero"`

	// VerifiedHeight is the height of the terminal verified header.
	VerifiedHeight uint64 `json:"verifiedHeight,omitempty"`

	// State is the current node state and its transition history.
	State StateRecord `json:"state"`

	// Cursors track in-progress phases so a restart resumes without
	// redoing work.
	Cursors Cursors `json:"cursors"`
}

// StateRecord captures node state and transition history.
type StateRecord struct {
	// Current is one of "BOOTING", "ACTIVE", "COMPLETE".
	Current string `json:"current"`

	// EnteredBooting / EnteredActive / EnteredComplete record when
	// each transition occurred. Zero for never-reached states.
	EnteredBooting  time.Time `json:"enteredBooting,omitzero"`
	EnteredActive   time.Time `json:"enteredActive,omitzero"`
	EnteredComplete time.Time `json:"enteredComplete,omitzero"`

	// HistoryDepth is the oldest block fully retained, set at the
	// ACTIVE → COMPLETE transition. Zero means unlimited (full
	// history retained).
	HistoryDepth uint64 `json:"historyDepth,omitempty"`
}

// Cursors track in-progress work so restarts don't redo it.
type Cursors struct {
	// WalkLastVerified is the highest header height the trust phase
	// has verified so far. Zero means the walk hasn't started.
	WalkLastVerified uint64 `json:"walkLastVerified,omitempty"`

	// AccountsPulled is the count of accounts the data phase has
	// successfully written. The list of remaining URLs is
	// reconstructed by the caller from the bootstrap config.
	AccountsPulled uint64 `json:"accountsPulled,omitempty"`

	// HistoryBackfillReached is the oldest block the post-ACTIVE
	// history backfill has retrieved. Zero means backfill hasn't
	// started.
	HistoryBackfillReached uint64 `json:"historyBackfillReached,omitempty"`
}

// ErrPinMismatch is returned by callers (not by this package
// directly) when the persisted PinnedValidatorSetHash doesn't match
// what the binary expects on startup. The package exports the
// sentinel for callers to use.
var ErrPinMismatch = errors.New("bootpersist: pinned validator-set hash mismatch — explicit migration required")

// ErrFormatMajor is returned when the persisted format major doesn't
// match this binary's. Indicates an incompatible artifact.
var ErrFormatMajor = errors.New("bootpersist: incompatible format major version")

// Load reads the artifact from `dir` and verifies it against the
// expected pinned validator-set hash. Returns os.ErrNotExist if the
// file is absent. Returns ErrPinMismatch if the persisted hash
// differs. Returns ErrFormatMajor for incompatible major versions.
func Load(dir string, expected [32]byte) (*Artifact, error) {
	a, err := Peek(dir)
	if err != nil {
		return nil, err
	}
	if a.PinnedValidatorSetHash != expected {
		return nil, ErrPinMismatch
	}
	return a, nil
}

// Peek reads and validates the artifact's format major but does not
// enforce the pin check. Callers that need to read the artifact's
// Network field before resolving the expected pin (e.g., during
// accumulated run startup) use this and enforce the pin themselves.
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

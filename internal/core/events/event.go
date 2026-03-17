// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package events

import (
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
)

type Event interface {
	IsEvent()
}

func (DidCommitBlock) IsEvent()          {}
func (DidSaveSnapshot) IsEvent()         {}
func (WillChangeGlobals) IsEvent()       {}
func (FatalError) IsEvent()              {}
func (StateDivergenceDetected) IsEvent() {}

type DidCommitBlock struct {
	Init  bool
	Index uint64
	Time  time.Time
	Major uint64
}

type DidSaveSnapshot struct {
	MinorIndex uint64
}

type WillChangeGlobals struct {
	New, Old *core.GlobalValues
}

type FatalError struct {
	Err error
}

func (e FatalError) Error() string { return e.Err.Error() }
func (e FatalError) Unwrap() error { return e.Err }

// StateDivergenceDetected is emitted when state hash mismatch is detected
// between validators, indicating potential consensus failure or state corruption.
type StateDivergenceDetected struct {
	// Round is the consensus round where divergence was detected.
	Round uint64
	// BlockIndex is the block index where divergence occurred.
	BlockIndex uint64
	// ExpectedHash is the local (expected) state hash.
	ExpectedHash [32]byte
	// ActualHash is the conflicting state hash received from another validator.
	ActualHash [32]byte
}

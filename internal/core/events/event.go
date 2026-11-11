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

	// Round and Epoch are the DAG-BFT leader round and committee epoch that
	// committed this block, recorded for the node's own bookkeeping.
	Round uint64
	Epoch uint64
}

type DidSaveSnapshot struct {
	MinorIndex uint64
}

// WillChangeGlobals announces the network globals an executor is about to
// apply.
//
// On a bus a partition provides (dagbftProvidesEventBus in
// cmd/accumulated/run/dagbft.go) only executed state may publish it. The
// submit gate (internal/node/dagbft/membership.go), the conductor's anchor
// gate (internal/core/crosschain/cadence.go) and the adapter's committee
// (pkg/consensus/adapter/executor_bridge.go) take the committee from it
// without checking a signature. exp/apiutil's router publishes a peer's answer
// only on a private bus of its own. TestWillChangeGlobalsPublishers enforces
// the set of files that may construct it.
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

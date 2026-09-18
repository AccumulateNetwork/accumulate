// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package nodestate

import (
	"strings"
	"sync"
)

// The node state a partition's services read to decide what they may answer
// (executor spec, "Sync", step 5). The join owns the machine; the sequencer,
// the staging snapshot and the history queries read it. A process running
// several networks — the simulator — hands each service its machine directly
// and does not use this, as it does not use the staging registry.
var (
	registryMu sync.RWMutex
	registry   = map[string]*Machine{}
)

// Register names a partition's node state.
func Register(partitionID string, m *Machine) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[strings.ToLower(partitionID)] = m
}

// For answers Register. Nil means nothing registered: a node that never
// joined, which is running and answers for itself.
func For(partitionID string) *Machine {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[strings.ToLower(partitionID)]
}

// Serving reports whether a partition's node may answer for data it does not
// hold — the sequencer's cache, the history, another node's join. A node with
// no machine registered has not joined and serves; one that is joining does
// not, and says so rather than answering from an empty cache (#4295).
func Serving(partitionID string) bool {
	m := For(partitionID)
	if m == nil {
		return true
	}
	switch m.State() {
	case StateActive, StateComplete:
		return true
	default:
		return false
	}
}

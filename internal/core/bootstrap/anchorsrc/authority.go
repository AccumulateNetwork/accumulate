// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package anchorsrc

import (
	"strings"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Set is one partition's validator set at one version of the network
// definition: which keys may sign for that partition, and how many distinct
// ones an anchor needs.
//
// It is a snapshot. A set is never edited once it is in an Authority's
// history, because an anchor signed under it must still be checkable after
// the network has moved on.
type Set struct {
	// Partition is the partition the set signs for.
	Partition string

	// Version is the network definition version this set was read from. An
	// anchor signature declares the version it was made under
	// (crosschain.signTransaction sets it from Network.Version), and the
	// version is hashed into the signature, so it names the set to check
	// against and cannot be moved afterwards.
	Version uint64

	// Threshold is how many distinct keys of this set must sign.
	Threshold uint64

	keys map[[32]byte]bool
}

// MaySign reports whether the key hash is one of this set's.
func (s *Set) MaySign(hash []byte) bool {
	if s == nil || len(hash) != 32 {
		return false
	}
	return s.keys[*(*[32]byte)(hash)]
}

// Size is how many keys are in the set.
func (s *Set) Size() int {
	if s == nil {
		return 0
	}
	return len(s.keys)
}

// Authority is the validator sets this node trusts, and the walk that extends
// them.
//
// **It starts from what the node already holds.** A signature is verified
// against a key, not against a root, so there is nothing to bootstrap: a
// restart holds the network definition it executed with, and a node started
// from genesis holds the genesis one. That is the whole of the answer to
// "there is nothing to verify the spine against" (#4301): the spine is not
// what the verifier reads its keys from, the node's own store is.
//
// **Churn is a walk.** A change to the network definition reaches a partition
// as a NetworkAccountUpdate carried inside a DirectoryAnchor
// (execute/v2/chain/directory_anchor.go), and that anchor is signed by a
// quorum of the PRECEDING set. So the walk is: verify an anchor under the set
// the node trusts now; only then apply the updates it carries; the next
// version becomes trustable. A signature declaring a version the walk has not
// reached is refused, never guessed at.
//
// Why the network definition and not <partition>/operators/1, which is what
// bootstrap-v3's anchorsrc read: on this line genesis writes EVERY node of
// EVERY partition into every partition's operator page and sets the page's
// threshold over all of them (node/daemon/init.go:196-233,266;
// node/genesis/bootstrap.go:544-562). A BVN's four validators can therefore
// never reach a threshold taken over twelve keys, so the page cannot answer
// "a quorum of that partition's validators". The protocol's own answer is the
// network definition: core.AnchorSigner admits a signer iff the definition
// says it is active on the source partition (internal/core/signer.go:42-48),
// and the quorum is GlobalValues.ValidatorThreshold for that partition
// (execute/v2/block/anchor_signatures.go:142). Both are read from
// <partition>.acme/network, which every partition holds its own copy of
// (genesis/bootstrap.go:254).
type Authority struct {
	mu sync.Mutex

	// values is the trusted globals at the version the walk has reached.
	values *core.GlobalValues

	// sets is the history: partition (lower case) → version → set. A version
	// is in here only because the node held it to begin with or because a
	// verified anchor carried the change that produced it.
	sets map[string]map[uint64]*Set
}

// FromStore reads the node's own network definition and globals and holds
// them as the trusted set.
//
// It reads local's OWN accounts — <local>.acme/network and
// <local>.acme/globals — and it must be called BEFORE anything is pulled. The
// pull overwrites accounts with a peer's, and an authority read after that is
// the peer's authority, which is the whole defect (#4301). This is the same
// discipline the join reads its executed block under (#4295).
func FromStore(db database.Viewer, local *url.URL) (*Authority, error) {
	if db == nil || local == nil {
		return nil, errors.BadRequest.With("anchorsrc.FromStore: a store and a partition are required")
	}
	get := func(account *url.URL, target interface{}) error {
		return db.View(func(batch *database.Batch) error {
			return batch.Account(account).Main().GetAs(target)
		})
	}

	values := new(core.GlobalValues)
	if err := values.LoadNetwork(local, get); err != nil {
		return nil, errors.UnknownError.WithFormat("read this node's own network definition: %w", err)
	}
	if err := values.LoadGlobals(local, get); err != nil {
		return nil, errors.UnknownError.WithFormat("read this node's own network globals: %w", err)
	}
	return FromValues(values)
}

// FromValues holds the given globals as the trusted set. It is what FromStore
// ends in, and it is how a test says what the node holds.
func FromValues(values *core.GlobalValues) (*Authority, error) {
	if values == nil || values.Network == nil || values.Globals == nil {
		return nil, errors.BadRequest.With("anchorsrc: a network definition and network globals are required")
	}
	// Not Copy: GlobalValues.Copy panics on the fields this does not need
	// (the oracle, the routing table), and neither the store nor a caller
	// has to supply them to say who may sign. The two it does need are
	// replaced wholesale by a walk, never edited, so sharing them is safe.
	a := &Authority{
		values: &core.GlobalValues{Network: values.Network, Globals: values.Globals},
		sets:   map[string]map[uint64]*Set{},
	}
	a.record(a.values)
	return a, nil
}

// Version is the network definition version the walk has reached.
func (a *Authority) Version() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.values.Network.Version
}

// BvnNames is the BVNs the trusted definition names. The join uses it to find
// a pool that holds the Directory's own anchors.
func (a *Authority) BvnNames() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.values.BvnNames()
}

// SetFor is the set that signs for a partition at a version.
//
// A version the walk has not reached is refused, and so is one older than the
// node started from: the node holds one definition, not a history, so there
// is no set to check an older signature against. Refusing is the safe answer
// in both directions — the anchors a join needs are the current ones, and one
// that cannot be checked is not a root.
func (a *Authority) SetFor(partition string, version uint64) (*Set, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	byVersion, ok := a.sets[strings.ToLower(partition)]
	if !ok {
		return nil, errors.NotFound.WithFormat("this node's network definition names no partition %s", partition)
	}
	set, ok := byVersion[version]
	if !ok {
		return nil, errors.NotFound.WithFormat(
			"no validator set for %s at network version %d: this node has walked to version %d",
			partition, version, a.values.Network.Version)
	}
	return set, nil
}

// Apply walks the authority forward over the updates an anchor carried.
//
// **The caller must have verified that anchor under the set this authority
// currently trusts, first.** That is what makes the walk safe: each operator
// change is anchored and signed by the preceding set, so applying the change
// an already-verified anchor carried extends the trust by exactly one signed
// step. Applying updates from an unverified anchor would let one peer name
// the validators, which is the defect this package exists to close.
//
// Updates that do not change the authority — the oracle, the routing table —
// are ignored. An update that does not parse is an error and nothing is
// applied.
func (a *Authority) Apply(updates []protocol.NetworkAccountUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	next := &core.GlobalValues{Network: a.values.Network, Globals: a.values.Globals}
	changed := false
	for _, u := range updates {
		body, ok := u.Body.(*protocol.WriteData)
		if !ok {
			continue // Only the network data accounts carry the authority
		}
		switch strings.ToLower(u.Name) {
		case protocol.Network:
			if err := next.ParseNetwork(body.Entry); err != nil {
				return errors.UnknownError.WithFormat("apply a network definition update: %w", err)
			}
			changed = true
		case protocol.Globals:
			if err := next.ParseGlobals(body.Entry); err != nil {
				return errors.UnknownError.WithFormat("apply a network globals update: %w", err)
			}
			changed = true
		}
	}
	if !changed {
		return nil
	}

	a.values = next
	a.record(next)
	return nil
}

// record adds a set per partition at the values' network version. The sets
// already in the history are left alone: an anchor signed under an earlier
// version is still checked against the set of its time.
func (a *Authority) record(values *core.GlobalValues) {
	version := values.Network.Version
	for _, p := range values.Network.Partitions {
		id := strings.ToLower(p.ID)
		set := &Set{
			Partition: p.ID,
			Version:   version,
			Threshold: values.ValidatorThreshold(p.ID),
			keys:      map[[32]byte]bool{},
		}
		for _, v := range values.Network.Validators {
			if v.IsActiveOn(p.ID) {
				set.keys[v.PublicKeyHash] = true
			}
		}
		if a.sets[id] == nil {
			a.sets[id] = map[uint64]*Set{}
		}
		a.sets[id][version] = set
	}
}

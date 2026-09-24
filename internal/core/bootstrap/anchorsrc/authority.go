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

// Set is one partition's validator set: which keys may sign for that
// partition, and how many distinct ones an anchor needs.
//
// There is one per partition and it is the set this node TRUSTS. A
// superseded set is not kept, because a set that is retired is usually
// retired for a reason and a quorum of retired keys must not be able to sign
// a new root (#4301, threat review F3).
type Set struct {
	// Partition is the partition the set signs for.
	Partition string

	// Version is the network definition version this set came from. It is
	// the floor on what a signature may declare and it is never a selector:
	// a signature declaring an OLDER version was made under a set this node
	// has moved past and is refused, and one declaring a newer version is
	// checked against this set like any other, because that is the only way
	// a change is ever crossed.
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

// Authority is the validator set this node trusts, per partition, and the
// step that moves it.
//
// **It starts from what the node already holds.** A signature is verified
// against a key, not against a root, so there is nothing to bootstrap: a
// restart holds the network definition it executed with, and a node started
// from genesis holds the genesis one. That is the whole of the answer to
// "there is nothing to verify the spine against" (#4301): the spine is not
// what the verifier reads its keys from, the node's own store is.
//
// **Membership in the trusted set decides, and the version a signature
// declares does not.** That is the executor's rule
// (msg_block_anchor.go, core.AnchorSigner, and its standing "TODO: Consider
// checking the version"), and it is the only rule that can cross a change:
// an anchor after a set change is signed by the set that came out of it, and
// so, on this line, is the anchor of the block that made the change. A
// verifier that demanded "the set of the time" would refuse every anchor
// from the moment the network moved and would never move again (#4301,
// review finding 1).
//
// **The step forward is a verified leaf, not a carrier.** There is no
// carrier: a change to dn.acme/network leaves the Directory as a
// messaging.NetworkUpdate past Vandenberg and never enters an anchor
// (block_end.go:791-793, network_accounts.go:128-131), so
// DirectoryAnchor.Updates is always empty here. What the join does instead
// is pull <partition>.acme/network and /globals as spine accounts, with a
// receipt that ends at a root a quorum signed and passes through the leaf
// the pulled body hashes to, and hand the result here (Update). The new
// definition inherits the anchor's quorum trust because it is bound to the
// anchor's root — which is what internal/fastsync/spine.go does in its own
// protocol.
//
// **Its limit, stated.** A change that turns over enough of the set that the
// new anchors cannot reach the OLD set's threshold cannot be crossed, and
// such a node must be re-seeded. The executor has the same limit for the
// same reason. An overlapping change — a validator in or out, a key rotated,
// a follower added — is crossed.
//
// Why the network definition and not <partition>/operators/1, which is what
// bootstrap-v3's anchorsrc read: on this line genesis writes EVERY node of
// EVERY partition into every partition's operator page and sets the page's
// threshold over all of them (node/daemon/init.go:196-233,266;
// node/genesis/bootstrap.go:544-562). A BVN's four validators can therefore
// never reach a threshold taken over twelve keys, so the page cannot answer
// "a quorum of that partition's validators". The protocol's own answer is
// the network definition: core.AnchorSigner admits a signer iff the
// definition says it is active on the source partition
// (internal/core/signer.go:42-48), and the quorum is
// GlobalValues.ValidatorThreshold for that partition
// (execute/v2/block/anchor_signatures.go:142). Both are read from
// <partition>.acme/network, which every partition holds its own copy of
// (genesis/bootstrap.go:254).
type Authority struct {
	mu sync.Mutex

	// values is the trusted globals.
	values *core.GlobalValues

	// sets is partition (lower case) → the one set that partition's anchors
	// are checked against. Replaced by Update, never accumulated.
	sets map[string]*Set
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
	values, err := readValues(db, local)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return FromValues(values)
}

// FromTrusted is the validator sets this node trusts: the definition it
// recorded under SystemData (Remember), or, when it has recorded none, its own
// accounts -- read before anything is pulled -- which it records then
// (#4438 F1). <partition>/network is an account the join's pull overwrites with
// a peer's before anything is proven, so a node that restarted between a pull
// and its match and read the account would trust the peer's validators; the
// record under SystemData is written only from the node's own execution and
// from a state that matched (executor spec, "Sync" §1: "a restart holds the
// network definition it executed with").
func FromTrusted(db *database.Database, local *url.URL) (*Authority, error) {
	values, ok, err := readTrusted(db, local)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if ok {
		return FromValues(values)
	}
	a, err := FromStore(db, local)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if err := a.Remember(db, local); err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return a, nil
}

// Remember records the sets this node trusts under SystemData, where no pull
// reaches: the record FromTrusted seeds a restart from. The caller records
// only what it has proven or executed.
func (a *Authority) Remember(db *database.Database, local *url.URL) error {
	id, ok := protocol.ParsePartitionUrl(local)
	if !ok {
		return errors.BadRequest.WithFormat("%v is not a partition", local)
	}
	network, err := a.values.Network.MarshalBinary()
	if err != nil {
		return errors.UnknownError.WithFormat("marshal the trusted network definition: %w", err)
	}
	globals, err := a.values.Globals.MarshalBinary()
	if err != nil {
		return errors.UnknownError.WithFormat("marshal the trusted globals: %w", err)
	}
	batch := db.Begin(true)
	defer batch.Discard()
	if err := batch.SystemData(id).TrustedNetwork().Put(network); err != nil {
		return errors.UnknownError.Wrap(err)
	}
	if err := batch.SystemData(id).TrustedGlobals().Put(globals); err != nil {
		return errors.UnknownError.Wrap(err)
	}
	return errors.UnknownError.Wrap(batch.Commit())
}

// readTrusted reads the record Remember wrote, and whether there is one.
func readTrusted(db *database.Database, local *url.URL) (*core.GlobalValues, bool, error) {
	id, ok := protocol.ParsePartitionUrl(local)
	if !ok {
		return nil, false, errors.BadRequest.WithFormat("%v is not a partition", local)
	}
	batch := db.Begin(false)
	defer batch.Discard()
	network, err := batch.SystemData(id).TrustedNetwork().Get()
	switch {
	case errors.Is(err, errors.NotFound):
		return nil, false, nil
	case err != nil:
		return nil, false, errors.UnknownError.Wrap(err)
	}
	globals, err := batch.SystemData(id).TrustedGlobals().Get()
	switch {
	case errors.Is(err, errors.NotFound):
		return nil, false, nil
	case err != nil:
		return nil, false, errors.UnknownError.Wrap(err)
	}
	if len(network) == 0 || len(globals) == 0 {
		return nil, false, nil
	}
	values := &core.GlobalValues{Network: new(protocol.NetworkDefinition), Globals: new(protocol.NetworkGlobals)}
	if err := values.Network.UnmarshalBinary(network); err != nil {
		return nil, false, errors.UnknownError.WithFormat("the trusted network definition: %w", err)
	}
	if err := values.Globals.UnmarshalBinary(globals); err != nil {
		return nil, false, errors.UnknownError.WithFormat("the trusted globals: %w", err)
	}
	return values, true, nil
}

func readValues(db database.Viewer, local *url.URL) (*core.GlobalValues, error) {
	if db == nil || local == nil {
		return nil, errors.BadRequest.With("anchorsrc: a store and a partition are required")
	}
	get := func(account *url.URL, target interface{}) error {
		return db.View(func(batch *database.Batch) error {
			return batch.Account(account).Main().GetAs(target)
		})
	}
	values := new(core.GlobalValues)
	if err := values.LoadNetwork(local, get); err != nil {
		return nil, errors.UnknownError.WithFormat("read %v's network definition: %w", local, err)
	}
	if err := values.LoadGlobals(local, get); err != nil {
		return nil, errors.UnknownError.WithFormat("read %v's network globals: %w", local, err)
	}
	return values, nil
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
		sets:   map[string]*Set{},
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

// SetFor is the set that signs for a partition — the one this node trusts,
// and the only one. There is no version argument on purpose: see Set.
func (a *Authority) SetFor(partition string) (*Set, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	set, ok := a.sets[strings.ToLower(partition)]
	if !ok {
		return nil, errors.NotFound.WithFormat("this node's network definition names no partition %s", partition)
	}
	return set, nil
}

// Update moves the trusted sets to a network definition the caller has
// VERIFIED, and reports whether anything changed.
//
// **The caller must have verified it as state**, not taken it from a peer:
// the definition is an account, and the join settles it like every other
// account — a receipt that is valid, that ends at a root a quorum of this
// partition's validators signed, and that passes through the leaf the pulled
// body hashes to (pull.Verify). That is what makes this an induction step
// and not a peer's assertion, and it is the only route by which the sets
// this node trusts ever move.
//
// A definition older than the trusted one is ignored. Anything else is
// taken, including a change that moves only the globals — the accept
// threshold is a ratio held there, and it decides the quorum.
func (a *Authority) Update(values *core.GlobalValues) bool {
	if values == nil || values.Network == nil || values.Globals == nil {
		return false
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if values.Network.Version < a.values.Network.Version {
		return false
	}
	next := &core.GlobalValues{Network: values.Network, Globals: values.Globals}
	if next.Network.Equal(a.values.Network) && next.Globals.Equal(a.values.Globals) {
		return false
	}
	a.values = next
	a.record(next)
	return true
}

// UpdateFrom reads the partition's network accounts out of a store the caller
// has filled with VERIFIED state, and moves the trusted sets to them. It is
// what the join calls once its spine has settled against an anchored root.
func (a *Authority) UpdateFrom(db database.Viewer, local *url.URL) (bool, error) {
	values, err := readValues(db, local)
	if err != nil {
		return false, errors.UnknownError.Wrap(err)
	}
	return a.Update(values), nil
}

// record replaces the set for each partition the definition names. What was
// there is dropped: a retired set is not a signer (see Set).
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
		a.sets[id] = set
	}
}

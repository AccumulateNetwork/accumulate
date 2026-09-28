// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	recordpkg "gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// A LeafDiff is one account whose leaf in this node's tree differs from the
// peer's as of a block: a zero hash is a leaf that side does not hold.
// Account is nil when neither side could name the key.
type LeafDiff struct {
	KeyHash [32]byte
	Account *url.URL
	Local   [32]byte
	Peer    [32]byte
}

func (d LeafDiff) String() string {
	name := fmt.Sprintf("key %x", d.KeyHash[:8])
	if d.Account != nil {
		name = d.Account.String()
	}
	return fmt.Sprintf("%s local %x peer %x", name, d.Local, d.Peer)
}

// maxLocateCalls bounds how many stored blocks one location asks a peer for.
// A tree that differs from the peer's in more places than this is not a pull
// bug to be located but a pull to be done again.
const maxLocateCalls = 4096

// locator walks a peer's BPT as of block v against this node's tree, stored
// block by stored block, descending only where the hashes differ (executor
// spec, "Sync", "The algorithm", step 3; #4441, #4442).
type locator struct {
	ctx       context.Context
	partition *url.URL
	peer      pull.Source
	local     localTree
	v         uint64
	calls     int
	diffs     []LeafDiff
}

// slotsOf is one side's positions under prefix, by index. A side with fewer
// than two keys under prefix holds no stored block there: none.
func (l *locator) peerSlots(prefix []byte) (map[byte]bpt.BlockSlot, error) {
	l.calls++
	if l.calls > maxLocateCalls {
		return nil, errors.NotAllowed.WithFormat("more than %d stored blocks differ", maxLocateCalls)
	}
	rec, err := l.peer.QueryBptBlock(l.ctx, l.partition, &api.BptBlockQuery{Prefix: prefix, ForHeight: l.v})
	switch {
	case errors.Is(err, errors.NotFound):
		return map[byte]bpt.BlockSlot{}, nil
	case err != nil:
		return nil, errors.UnknownError.WithFormat("the peer's stored block under %x as of %d: %w", prefix, l.v, err)
	}
	if rec.Block > l.v {
		return nil, errors.Conflict.WithFormat("the peer answered as of block %d, not %d", rec.Block, l.v)
	}
	out := map[byte]bpt.BlockSlot{}
	for _, s := range rec.Slots {
		if s != nil {
			out[byte(s.Index)] = bpt.BlockSlot{Index: byte(s.Index), Branch: s.Branch, KeyHash: s.KeyHash, Hash: s.Hash}
		}
	}
	return out, nil
}

func (l *locator) localSlots(prefix []byte) (map[byte]bpt.BlockSlot, error) {
	b, err := l.local.GetStoredBlock(prefix)
	switch {
	case errors.Is(err, errors.NotFound):
		return map[byte]bpt.BlockSlot{}, nil
	case err != nil:
		return nil, errors.UnknownError.WithFormat("this node's stored block under %x: %w", prefix, err)
	}
	out := map[byte]bpt.BlockSlot{}
	for _, s := range b.Slots {
		out[s.Index] = s
	}
	return out, nil
}

// compare descends under prefix, where the two sides' hashes differ.
func (l *locator) compare(prefix []byte) error {
	peer, err := l.peerSlots(prefix)
	if err != nil {
		return err
	}
	local, err := l.localSlots(prefix)
	if err != nil {
		return err
	}
	for i := 0; i < 256; i++ {
		p, inPeer := peer[byte(i)]
		m, inLocal := local[byte(i)]
		if !inPeer && !inLocal || inPeer && inLocal && p.Hash == m.Hash {
			continue
		}
		sub := append(append([]byte{}, prefix...), byte(i))
		if inPeer && inLocal && p.Branch && m.Branch {
			if err := l.compare(sub); err != nil {
				return err
			}
			continue
		}
		// The two sides are shaped differently here: every leaf under the
		// position, on each side, and the difference between them.
		pl, err := l.leaves(sub, p, inPeer, l.peerSlots)
		if err != nil {
			return err
		}
		ml, err := l.leaves(sub, m, inLocal, l.localSlots)
		if err != nil {
			return err
		}
		for k, h := range pl {
			if ml[k] != h {
				l.diffs = append(l.diffs, LeafDiff{KeyHash: k, Local: ml[k], Peer: h})
			}
		}
		for k, h := range ml {
			if _, ok := pl[k]; !ok {
				l.diffs = append(l.diffs, LeafDiff{KeyHash: k, Local: h})
			}
		}
	}
	return nil
}

// leaves is every leaf under one side's slot at prefix, by key hash.
func (l *locator) leaves(prefix []byte, s bpt.BlockSlot, ok bool, slots func([]byte) (map[byte]bpt.BlockSlot, error)) (map[[32]byte][32]byte, error) {
	out := map[[32]byte][32]byte{}
	if !ok {
		return out, nil
	}
	if !s.Branch {
		out[s.KeyHash] = s.Hash
		return out, nil
	}
	under, err := slots(prefix)
	if err != nil {
		return nil, err
	}
	for i, t := range under {
		m, err := l.leaves(append(append([]byte{}, prefix...), i), t, true, slots)
		if err != nil {
			return nil, err
		}
		for k, h := range m {
			out[k] = h
		}
	}
	return out, nil
}

// name finds the account a key hash is the leaf of: from this node's tree,
// or from the peer's as of v.
func (l *locator) name(k [32]byte) *url.URL {
	start, ok := after(k)
	if !ok {
		return nil
	}
	if pairs, _, err := l.local.GetRange(start, 1); err == nil && len(pairs) == 1 && [32]byte(pairs[0].Key.Hash()) == k {
		if u := accountOfKey(pairs[0].Key); u != nil {
			return u
		}
	}
	page, err := l.peer.QueryBptPage(l.ctx, l.partition, &api.BptPageQuery{StartHash: start, Count: 1, ForHeight: l.v})
	if err == nil && len(page.Entries) == 1 && page.Entries[0] != nil && page.Entries[0].KeyHash == k {
		return page.Entries[0].Account
	}
	return nil
}

// after is the key a range starts after to begin at k: the tree is ranged in
// descending key order, so the key one above it.
func after(k [32]byte) ([32]byte, bool) {
	for i := 31; i >= 0; i-- {
		k[i]++
		if k[i] != 0 {
			return k, true
		}
	}
	return k, false
}

func accountOfKey(k *recordpkg.Key) *url.URL {
	if k.Len() < 2 || k.Get(0) != "Account" {
		return nil
	}
	u, _ := k.Get(1).(*url.URL)
	return u
}

// locateAt walks the peers' BPTs as of block v against this node's tree, the
// first that answers, and returns every account whose leaf differs and the
// peer that answered.
func locateAt(ctx context.Context, partition *url.URL, srcs []pull.Source, local localTree, v uint64) ([]LeafDiff, string, error) {
	var last error = errors.NotFound.With("no peer to locate against")
	for _, src := range srcs {
		l := &locator{ctx: ctx, partition: partition, peer: src, local: local, v: v}
		err := l.compare(nil)
		if err != nil {
			last = err
			continue
		}
		for i := range l.diffs {
			l.diffs[i].Account = l.name(l.diffs[i].KeyHash)
		}
		return l.diffs, sourceKey(src), nil
	}
	return nil, "", last
}

// localTree is this node's tree as the locator reads it: now, or as of a
// block it retains history for.
type localTree interface {
	GetStoredBlock(prefix []byte) (*bpt.StoredBlock, error)
	GetRange(start [32]byte, count int) ([]bpt.KeyValuePair, [32]byte, error)
}

// treeAt is this node's tree as of block height, whose root was root, from the
// BPT history it retains.
type treeAt struct {
	tree   *bpt.BPT
	height uint64
	root   [32]byte
}

func (t treeAt) GetStoredBlock(prefix []byte) (*bpt.StoredBlock, error) {
	return t.tree.GetStoredBlockAt(t.height, t.root, prefix)
}

func (t treeAt) GetRange(start [32]byte, count int) ([]bpt.KeyValuePair, [32]byte, error) {
	return t.tree.GetRangeAt(t.height, t.root, start, count)
}

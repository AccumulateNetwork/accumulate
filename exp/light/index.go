// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package light

import (
	"sort"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type (
	ChainIndexUint = ChainIndex[uint64]
	ChainIndexTime = ChainIndex[time.Time]
)

var (
	newChainIndexUint = newChainIndex[uint64]
	newChainIndexTime = newChainIndex[time.Time]
)

type ChainIndex[T any] struct {
	*merkle.ChainIndex
	parent record.Record
}

func newChainIndex[T any](parent record.Record, logger log.Logger, store record.Store, key *record.Key, name string) *ChainIndex[T] {
	return &ChainIndex[T]{merkle.NewChainIndex(logger, store, key), parent}
}

func (c *ChainIndex[T]) Append(key T, index uint64) error {
	return c.ChainIndex.Append(record.NewKey(key), index)
}

func (c *ChainIndex[T]) Find(key T) merkle.ChainSearchResult {
	return c.ChainIndex.Find(record.NewKey(key))
}

func (c *ChainIndex[T]) FindIndexEntryBefore(key T) (*protocol.IndexEntry, error) {
	return c.resolveEntry(c.Find(key).Before())
}

func (c *ChainIndex[T]) FindExactIndexEntry(key T) (*protocol.IndexEntry, error) {
	return c.resolveEntry(c.Find(key).Exact())
}

func (c *ChainIndex[T]) FindIndexEntryAfter(key T) (*protocol.IndexEntry, error) {
	return c.resolveEntry(c.Find(key).After())
}

func (c *ChainIndex[T]) Chain() *database.Chain2 {
	switch p := c.parent.(type) {
	case *IndexDBAccountChain:
		d, err := p.parent.parent.parent.
			Account(c.Key().Get(3).(*url.URL)).
			ChainByName(c.Key().Get(5).(string))
		if err != nil {
			panic(err)
		}
		return d
	case *IndexDBPartitionAnchors:
		if c.Key().Get(5) == "Received" {
			return p.parent.parent.parent.
				Account(c.Key().Get(3).(*url.URL).JoinPath(protocol.AnchorPool)).
				MainChain()
		}
	}
	panic(errors.BadRequest.WithFormat("cannot get chain from %v", c.Key()))
}

func (c *ChainIndex[T]) resolveEntry(r merkle.ChainSearchResult2) (*protocol.IndexEntry, error) {
	i, err := r.Index()
	if err != nil {
		return nil, err
	}
	d := c.Chain()
	if d.Type() != merkle.ChainTypeIndex {
		d = d.Index()
	}
	e := new(protocol.IndexEntry)
	err = d.EntryAs(int64(i), e)
	return e, err
}

// FindEntry returns the index and value of the first entry for which the
// predicate is true, assuming it is false for some (possibly empty) prefix and
// true for the remainder of the entries.
func FindEntry[T any](entries []T, fn func(T) bool) (int, T, bool) {
	i := sort.Search(len(entries), func(i int) bool { return fn(entries[i]) })
	if i >= len(entries) {
		var z T
		return 0, z, false
	}
	return i, entries[i], true
}

// ByIndexSource searches for an index entry with [protocol.IndexEntry.Source]
// equal to or greater than the given value.
func ByIndexSource(source uint64) func(*protocol.IndexEntry) bool {
	return func(e *protocol.IndexEntry) bool { return e.Source >= source }
}

// ByIndexBlock searches for an index entry with
// [protocol.IndexEntry.BlockIndex] equal to or greater than the given value.
func ByIndexBlock(block uint64) func(*protocol.IndexEntry) bool {
	return func(e *protocol.IndexEntry) bool { return e.BlockIndex >= block }
}

// ByIndexTime searches for an index entry with [protocol.IndexEntry.BlockTime]
// equal to or greater than the given value.
func ByIndexTime(t time.Time) func(*protocol.IndexEntry) bool {
	return func(e *protocol.IndexEntry) bool { return e.BlockTime.After(t) }
}

// ByIndexRootIndexIndex searches for an index entry with
// [protocol.IndexEntry.RootIndexIndex] equal to or greater than the given
// value.
func ByIndexRootIndexIndex(rootIndex uint64) func(*protocol.IndexEntry) bool {
	return func(e *protocol.IndexEntry) bool { return e.RootIndexIndex >= rootIndex }
}

// ByAnchorBlock searches for an anchor entry with
// [protocol.AnchorMetadata.MinorBlockIndex] equal to or greater than the given
// value.
func ByAnchorBlock(block uint64) func(*AnchorMetadata) bool {
	return func(a *AnchorMetadata) bool { return a.Anchor.MinorBlockIndex >= block }
}

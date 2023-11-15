// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package memory

import (
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// ChangeSet is a key-value change set.
type ChangeSet struct {
	mu       sync.RWMutex       //
	entries  map[[32]byte]Entry //
	getfn    GetFunc            // Gets an entry from the previous layer
	commitfn CommitFunc         // Commits changes into the previous layer
	discard  DiscardFunc        // Cleans up after discard
	prefix   *record.Key        // Prefix to apply to keys
}

// Entry is a [ChangeSet] entry.
type Entry struct {
	Key    *record.Key
	Value  []byte
	Delete bool
}

type GetFunc = func(*record.Key) ([]byte, error)
type CommitFunc = func(map[[32]byte]Entry) error
type DiscardFunc = func()

var _ keyvalue.ChangeSet = (*ChangeSet)(nil)

func NewChangeSet(prefix *record.Key, get GetFunc, commit CommitFunc, discard DiscardFunc) *ChangeSet {
	c := new(ChangeSet)
	c.getfn = get
	c.commitfn = commit
	c.discard = discard
	c.prefix = prefix
	return c
}

// Commit commits pending changes to the parent store.
func (c *ChangeSet) Commit() error {
	c.mu.Lock()
	entries := c.entries
	c.entries = nil
	c.mu.Unlock()

	if c.discard != nil {
		defer c.discard()
	}

	// Is there anything to do?
	if len(entries) == 0 {
		return nil
	}

	// Commit to the parent
	return c.commitfn(entries)
}

// Discard discards pending changes.
func (c *ChangeSet) Discard() {
	c.mu.Lock()
	c.entries = nil
	c.mu.Unlock()

	if c.discard != nil {
		c.discard()
	}
}

// Begin begins a nested change set.
func (c *ChangeSet) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	var commit CommitFunc
	if writable {
		if c.commitfn == nil {
			panic("attempted to create a writable change set from a read-only one")
		}
		commit = c.putAll
	}
	return NewChangeSet(prefix, c.Get, commit, nil)
}

// putAll stores a set of entries.
func (c *ChangeSet) putAll(entries map[[32]byte]Entry) error {
	if len(entries) == 0 {
		return nil
	}
	if c.commitfn == nil {
		return errors.NotAllowed.With("change set is not writable")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.entries == nil {
		// If the change set has no prefix we can just claim the map we're
		// passed
		if c.prefix == nil {
			c.entries = entries
			return nil
		}

		c.entries = make(map[[32]byte]Entry, len(entries))
	}

	for _, e := range entries {
		key := c.prefix.AppendKey(e.Key)
		c.entries[key.Hash()] = Entry{key, e.Value, false}
	}
	return nil
}

// Get loads a value.
func (c *ChangeSet) Get(key *record.Key) ([]byte, error) {
	// Prefix the key
	key = c.prefix.AppendKey(key)

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Get locally
	entry, ok := c.entries[key.Hash()]
	if ok {
		if entry.Delete {
			return nil, errors.NotFound.WithFormat("%v not found", key)
		}
		return entry.Value, nil
	}

	// Get from parent
	if c.getfn != nil {
		return c.getfn(key)
	}

	// Not found
	return nil, errors.NotFound.WithFormat("%v not found", key)
}

// Put stores a value.
func (c *ChangeSet) Put(key *record.Key, value []byte) error {
	return c.putEntry(Entry{key, value, false})
}

// Delete deletes a key-value pair.
func (c *ChangeSet) Delete(key *record.Key) error {
	return c.putEntry(Entry{key, nil, true})
}

func (c *ChangeSet) putEntry(entry Entry) error {
	if c.commitfn == nil {
		return errors.NotAllowed.With("change set is not writable")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Make sure the map is not nil
	if c.entries == nil {
		c.entries = map[[32]byte]Entry{}
	}

	// Prefix the key
	entry.Key = c.prefix.AppendKey(entry.Key)

	// Add or update the entry
	c.entries[entry.Key.Hash()] = entry
	return nil
}

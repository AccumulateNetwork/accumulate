// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package memory

import (
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// ChangeSet is a key-value change set.
type ChangeSet struct {
	mu      sync.RWMutex
	entries map[[32]byte]Entry
	opts    ChangeSetOptions
}

// Entry is a [ChangeSet] entry.
type Entry struct {
	Key    *record.Key
	Value  []byte
	Delete bool
}

type GetFunc = func(*record.Key) ([]byte, error)
type CommitFunc = func(map[[32]byte]Entry) error
type ForEachFunc = func(func(*record.Key, []byte) error) error
type DiscardFunc = func()

var _ keyvalue.ChangeSet = (*ChangeSet)(nil)

type ChangeSetOptions struct {
	Prefix  *record.Key // Prefix to apply to keys
	Get     GetFunc     // Gets an entry from the previous layer
	Commit  CommitFunc  // Commits changes into the previous layer
	ForEach ForEachFunc // Iterates over each entry
	Discard DiscardFunc // Cleans up after discard
}

func NewChangeSet(opts ChangeSetOptions) *ChangeSet {
	return &ChangeSet{opts: opts}
}

// Commit commits pending changes to the parent store.
func (c *ChangeSet) Commit() error {
	c.mu.Lock()
	entries := c.entries
	c.entries = nil
	c.mu.Unlock()

	if c.opts.Discard != nil {
		defer c.opts.Discard()
	}

	// Is there anything to do?
	if len(entries) == 0 {
		return nil
	}

	if c.opts.Commit == nil {
		panic("attempted to commit entries from a read-only change set")
	}

	// Commit to the parent
	return c.opts.Commit(entries)
}

// Discard discards pending changes.
func (c *ChangeSet) Discard() {
	c.mu.Lock()
	c.entries = nil
	c.mu.Unlock()

	if c.opts.Discard != nil {
		c.opts.Discard()
	}
}

// Begin begins a nested change set.
func (c *ChangeSet) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	var commit CommitFunc
	if writable {
		if c.opts.Commit == nil {
			panic("attempted to create a writable change set from a read-only one")
		}
		commit = c.putAll
	}
	return NewChangeSet(ChangeSetOptions{
		Prefix:  c.opts.Prefix,
		Get:     c.Get,
		Commit:  commit,
		ForEach: c.ForEach,
	})
}

// putAll stores a set of entries.
func (c *ChangeSet) putAll(entries map[[32]byte]Entry) error {
	if len(entries) == 0 {
		return nil
	}
	if c.opts.Commit == nil {
		return errors.NotAllowed.With("change set is not writable")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.entries == nil {
		// If the change set has no prefix we can just claim the map we're
		// passed
		if c.opts.Prefix == nil {
			c.entries = entries
			return nil
		}

		c.entries = make(map[[32]byte]Entry, len(entries))
	}

	for _, e := range entries {
		key := c.opts.Prefix.AppendKey(e.Key)
		c.entries[key.Hash()] = Entry{key, e.Value, e.Delete}
	}
	return nil
}

// Get loads a value.
func (c *ChangeSet) Get(key *record.Key) ([]byte, error) {
	// Prefix the key
	key = c.opts.Prefix.AppendKey(key)

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Get locally
	entry, ok := c.entries[key.Hash()]
	if ok {
		if entry.Delete {
			return nil, (*database.NotFoundError)(key)
		}
		return entry.Value, nil
	}

	// Get from parent
	if c.opts.Get != nil {
		return c.opts.Get(key)
	}

	// Not found
	return nil, (*database.NotFoundError)(key)
}

// Put stores a value.
func (c *ChangeSet) Put(key *record.Key, value []byte) error {
	return c.putEntry(Entry{key, value, false})
}

// Delete deletes a key-value pair.
func (c *ChangeSet) Delete(key *record.Key) error {
	return c.putEntry(Entry{key, nil, true})
}

func (c *ChangeSet) ForEach(fn func(*record.Key, []byte) error) error {
	if c.opts.ForEach == nil {
		return errors.NotAllowed.With("cannot iterate over database")
	}

	for _, e := range c.entries {
		if e.Delete {
			continue
		}
		err := fn(e.Key, e.Value)
		if err != nil {
			return err
		}
	}

	return c.opts.ForEach(func(key *record.Key, value []byte) error {
		// Skip overridden entries
		if _, ok := c.entries[key.Hash()]; ok {
			return nil
		}

		return fn(key, value)
	})
}

func (c *ChangeSet) putEntry(entry Entry) error {
	if c.opts.Commit == nil {
		return errors.NotAllowed.With("change set is not writable")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Make sure the map is not nil
	if c.entries == nil {
		c.entries = map[[32]byte]Entry{}
	}

	// Prefix the key
	entry.Key = c.opts.Prefix.AppendKey(entry.Key)

	// Add or update the entry
	c.entries[entry.Key.Hash()] = entry
	return nil
}

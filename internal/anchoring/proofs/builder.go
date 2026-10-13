// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package proofs is the proof builder of Bitcoin anchoring (anchoring.md
// §2.5, #4563): it reads a partition's block ledgers and keeps, for every entry
// of the configured accounts, the receipt from the entry to the Directory root
// that committed it, so the evidence package is ready when that root is
// anchored.
package proofs

import (
	"encoding/json"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/anchoring"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Builder keeps proofs for the entries of Accounts, read from Partition's block
// ledgers. Store holds its progress, the proofs, and the entries whose
// Directory root has not arrived yet.
type Builder struct {
	Network   string
	Partition string // the partition's ID
	Database  database.Viewer
	// Directory resolves the Directory's database, which binds a partition
	// root to a Directory root. Nil on the Directory itself.
	Directory func() (database.Viewer, error)
	Accounts  []*url.URL
	Store     keyvalue.Beginner
}

// pending is an entry whose receipt to the partition's root exists but whose
// Directory root has not been committed yet; it is retried on every run.
type pending struct {
	Account *url.URL `json:"account"`
	Chain   string   `json:"chain"`
	Index   uint64   `json:"index"`
}

func keyProgress() *record.Key { return record.NewKey("Progress") }
func keyProof(account *url.URL, chain string, index uint64) *record.Key {
	return record.NewKey("Proof", account.String(), chain, index)
}
func keyByHash(hash []byte) *record.Key { return record.NewKey("ByHash", hash) }
func keyPending(account *url.URL, chain string, index uint64) *record.Key {
	return record.NewKey("Pending", account.String(), chain, index)
}

func (b *Builder) url() config.NetworkUrl {
	return config.NetworkUrl{URL: protocol.PartitionUrl(b.Partition)}
}

func (b *Builder) wants(u *url.URL) bool {
	for _, a := range b.Accounts {
		if a.Equal(u) {
			return true
		}
	}
	return false
}

// Run processes every block ledger written since the last run, then retries
// the pending entries. It returns the number of proofs completed.
func (b *Builder) Run() (int, error) {
	var done int
	err := b.Database.View(func(batch *database.Batch) error {
		if b.Directory == nil {
			var err error
			done, err = b.run(batch, batch)
			return err
		}
		dir, err := b.Directory()
		if err != nil {
			return errors.UnknownError.WithFormat("open the directory: %w", err)
		}
		return dir.View(func(dn *database.Batch) error {
			var err error
			done, err = b.run(batch, dn)
			return err
		})
	})
	return done, err
}

func (b *Builder) run(batch, dn *database.Batch) (int, error) {
	ledger := batch.Account(b.url().Ledger())
	var sys *protocol.SystemLedger
	err := ledger.Main().GetAs(&sys)
	if err != nil {
		return 0, errors.UnknownError.WithFormat("load the system ledger: %w", err)
	}

	st := b.Store.Begin(nil, true)
	defer st.Discard()

	var from uint64
	if v, err := st.Get(keyProgress()); err == nil {
		_ = json.Unmarshal(v, &from)
	}

	var done int
	var todo []pending
	for block := from + 1; block <= sys.Index; block++ {
		bl, err := ledger.BlockLedger(block).Get()
		switch {
		case err == nil:
		case errors.Is(err, errors.NotFound):
			continue // the block predates block ledgers
		default:
			return 0, errors.UnknownError.WithFormat("load block ledger %d: %w", block, err)
		}
		for _, e := range bl.Entries {
			if b.wants(e.Account) {
				todo = append(todo, pending{e.Account, e.Chain, e.Index})
			}
		}
	}
	err = st.ForEach(func(k *record.Key, v []byte) error {
		if k.Len() == 0 || k.Get(0) != "Pending" {
			return nil
		}
		var p pending
		if err := json.Unmarshal(v, &p); err != nil {
			return err
		}
		todo = append(todo, p)
		return nil
	})
	if err != nil {
		return 0, errors.UnknownError.WithFormat("list pending: %w", err)
	}

	for _, p := range todo {
		pkg, err := b.prove(batch, dn, p)
		if err != nil {
			return 0, err
		}
		pk := keyPending(p.Account, p.Chain, p.Index)
		if pkg == nil {
			v, _ := json.Marshal(p)
			if err := st.Put(pk, v); err != nil {
				return 0, err
			}
			continue
		}
		v, err := json.Marshal(pkg)
		if err != nil {
			return 0, err
		}
		if err := st.Put(keyProof(p.Account, p.Chain, p.Index), v); err != nil {
			return 0, err
		}
		ref, _ := json.Marshal(p)
		if err := st.Put(keyByHash(pkg.Entry.Hash), ref); err != nil {
			return 0, err
		}
		if _, err := st.Get(pk); err == nil {
			if err := st.Delete(pk); err != nil {
				return 0, err
			}
		}
		done++
	}

	v, _ := json.Marshal(sys.Index)
	if err := st.Put(keyProgress(), v); err != nil {
		return 0, err
	}
	return done, st.Commit()
}

// prove builds the entry's receipt to the Directory root, or returns nil when
// the Directory has not committed the partition's anchor yet.
func (b *Builder) prove(batch, dn *database.Batch, p pending) (*anchoring.Package, error) {
	chain, err := batch.Account(p.Account).ChainByName(p.Chain)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("%v chain %s: %w", p.Account, p.Chain, err)
	}
	hash, err := chain.Entry(int64(p.Index))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("%v %s entry %d: %w", p.Account, p.Chain, p.Index, err)
	}

	// Entry to the partition's root chain anchor, by the stored positions
	// (database.md, "Proofs are read, not searched").
	rootEntry, _, extend, err := indexing.ReceiptForChainIndex(b.url(), batch, chain, int64(p.Index), nil)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("%v %s entry %d to the root: %w", p.Account, p.Chain, p.Index, err)
	}

	pkg := &anchoring.Package{
		Network: b.Network,
		Account: p.Account,
		Entry:   anchoring.Entry{Chain: p.Chain, Index: p.Index, Hash: hash},
	}

	if b.Directory == nil {
		pkg.Receipt = extend
		pkg.Directory = anchoring.Directory{Height: rootEntry.BlockIndex, Root: extend.Anchor}
		return pkg, nil
	}

	// The Directory's anchor(P)-root chain holds exactly the root anchors the
	// partition has sent; bind to the Directory root that committed this one.
	received := dn.Account(protocol.DnUrl().JoinPath(protocol.AnchorPool)).AnchorChain(b.Partition).Root()
	at, err := received.IndexOf(extend.Anchor)
	switch {
	case err == nil:
	case errors.Is(err, errors.NotFound):
		return nil, nil // not anchored yet
	default:
		return nil, errors.UnknownError.WithFormat("search anchor(%s)-root: %w", b.Partition, err)
	}
	dnEntry, _, bind, err := indexing.ReceiptForChainIndex(config.NetworkUrl{URL: protocol.DnUrl()}, dn, received, at, nil)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("bind to a directory root: %w", err)
	}
	joined, err := extend.Combine(bind)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("join the two hops: %w", err)
	}
	pkg.Receipt = joined
	pkg.Directory = anchoring.Directory{Height: dnEntry.BlockIndex, Root: joined.Anchor}
	return pkg, nil
}

// Get returns the package for an entry, or NotFound.
func (b *Builder) Get(account *url.URL, chain string, index uint64) (*anchoring.Package, error) {
	st := b.Store.Begin(nil, false)
	defer st.Discard()
	return load(st, keyProof(account, chain, index))
}

// ByHash returns the package for an entry by its hash, or NotFound.
func (b *Builder) ByHash(hash []byte) (*anchoring.Package, error) {
	st := b.Store.Begin(nil, false)
	defer st.Discard()
	v, err := st.Get(keyByHash(hash))
	if err != nil {
		return nil, err
	}
	var p pending
	if err := json.Unmarshal(v, &p); err != nil {
		return nil, err
	}
	return load(st, keyProof(p.Account, p.Chain, p.Index))
}

func load(st keyvalue.ChangeSet, key *record.Key) (*anchoring.Package, error) {
	v, err := st.Get(key)
	if err != nil {
		return nil, err
	}
	pkg := new(anchoring.Package)
	if err := json.Unmarshal(v, pkg); err != nil {
		return nil, fmt.Errorf("decode %v: %w", key, err)
	}
	return pkg, nil
}

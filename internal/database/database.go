// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"fmt"
	"io"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const markPower = 8

// Database is an Accumulate database.
type Database struct {
	store       keyvalue.Beginner
	logger      log.Logger
	nextBatchId int64
	observer    Observer
}

// New creates a new database using the given key-value store.
func New(store keyvalue.Beginner, logger log.Logger) *Database {
	d := new(Database)
	d.store = store
	d.observer = unsetObserver{}

	if logger != nil {
		d.logger = logger.With("module", "database")
	}

	return d
}

func OpenInMemory(logger log.Logger) *Database {
	store := memory.New(nil)
	return New(store, logger)
}

func OpenBadger(filepath string, logger log.Logger) (*Database, error) {
	store, err := badger.New(filepath)
	if err != nil {
		return nil, err
	}
	return New(store, logger), nil
}

func OpenLevelDB(filepath string, logger log.Logger) (*Database, error) {
	store, err := leveldb.Open(filepath)
	if err != nil {
		return nil, err
	}
	return New(store, logger), nil
}

// Open opens a key-value store and creates a new database with it.
func Open(cfg *config.Config, logger log.Logger) (*Database, error) {
	switch cfg.Accumulate.Storage.Type {
	case config.MemoryStorage:
		return OpenInMemory(logger), nil

	case config.BadgerStorage:
		return OpenBadger(config.MakeAbsolute(cfg.RootDir, cfg.Accumulate.Storage.Path), logger)

	case config.LevelDBStorage:
		return OpenLevelDB(config.MakeAbsolute(cfg.RootDir, cfg.Accumulate.Storage.Path), logger)

	default:
		return nil, fmt.Errorf("unknown storage format %q", cfg.Accumulate.Storage.Type)
	}
}

// Store returns the underlying key-value store. Store may return an error in
// the future.
func (d *Database) Store() (keyvalue.Beginner, error) {
	return d.store, nil
}

// SetObserver sets the database observer.
func (d *Database) SetObserver(observer Observer) {
	if observer == nil {
		observer = unsetObserver{}
	}
	d.observer = observer
}

// Close closes the database and the key-value store.
func (d *Database) Close() error {
	if c, ok := d.store.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (b *Batch) GetMinorRootChainAnchor(describe *config.Describe) ([]byte, error) {
	ledger := b.Account(describe.NodeUrl(protocol.Ledger))
	chain, err := ledger.RootChain().Get()
	if err != nil {
		return nil, err
	}
	return chain.Anchor(), nil
}

type Observer interface {
	DidChangeAccount(batch *Batch, account *Account) (hash.Hasher, error)
}

type unsetObserver struct{}

func (unsetObserver) DidChangeAccount(batch *Batch, account *Account) (hash.Hasher, error) {
	// For read-only proof generation, compute minimal account state hash
	// This is safe because we're not modifying the database
	if !batch.writable {
		return computeReadOnlyAccountHash(batch, account)
	}
	return nil, errors.NotReady.WithFormat("cannot modify account - observer is not set")
}

// computeReadOnlyAccountHash computes a minimal account state hash for read-only proof generation
// This implements the same 4-component structure as the full observer but simplified for read-only access
func computeReadOnlyAccountHash(batch *Batch, account *Account) (hash.Hasher, error) {
	var hasher hash.Hasher
	
	// Component 1: Main state
	main, err := account.Main().Get()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, err
	}
	if main != nil {
		data, err := main.MarshalBinary()
		if err != nil {
			return nil, err
		}
		hasher.AddBytes(data)
	} else {
		hasher.AddBytes(nil)
	}
	
	// Component 2: Secondary state (directory + events)
	var secondaryHasher hash.Hasher
	
	// Add directory
	var dirHasher hash.Hasher
	dir, err := account.Directory().Get()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, err
	}
	for _, u := range dir {
		dirHasher.AddUrl(u)
	}
	secondaryHasher.AddValue(dirHasher)
	
	// Add scheduled events for system ledger accounts
	u := account.Url()
	if _, ok := protocol.ParsePartitionUrl(u); ok && u.PathEqual(protocol.Ledger) {
		eventsHash, err := account.Events().BPT().GetRootHash()
		if err == nil && eventsHash != [32]byte{} {
			secondaryHasher.AddHash2(eventsHash)
		}
	}
	hasher.AddValue(secondaryHasher)
	
	// Component 3: Chains
	var chainsHasher hash.Hasher
	chains, err := account.Chains().Get()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, err
	}
	for _, chainMeta := range chains {
		chain, err := account.GetChainByName(chainMeta.Name)
		if err != nil {
			continue
		}
		state := chain.CurrentState()
		if state.Count == 0 {
			chainsHasher.AddHash(new([32]byte))
		} else {
			chainsHasher.AddHash((*[32]byte)(state.Anchor()))
		}
	}
	hasher.AddValue(chainsHasher)
	
	// Component 4: Pending transactions
	var pendingHasher hash.Hasher
	pending, err := account.Pending().Get()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, err
	}
	// Add pending transaction IDs
	for _, txid := range pending {
		// For read-only, just add the transaction ID hash
		// We can't access the full transaction details without more context
		pendingHasher.AddTxID(txid)
	}
	
	// Check if account is a key page and add its authority's pending transactions
	if main != nil {
		if page, ok := main.(*protocol.KeyPage); ok {
			authPending, err := batch.Account(page.GetAuthority()).Pending().Get()
			if err == nil {
				for _, txid := range authPending {
					pendingHasher.AddTxID(txid)
				}
			}
		}
	}
	hasher.AddValue(pendingHasher)
	
	return hasher, nil
}

package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

type stateCache struct {
	*config.Network
	logger logging.OptionalLogger
	txType protocol.TransactionType
	txHash types.Bytes32

	state      ProcessTransactionState
	batch      *database.Batch
	operations []stateOperation
	chains     map[[32]byte]protocol.Account
	indices    map[[32]byte]*writeIndex

	Pretend bool
}

func newStateCache(net *config.Network, txtype protocol.TransactionType, txid [32]byte, batch *database.Batch) *stateCache {
	c := new(stateCache)
	c.Network = net
	c.txType = txtype
	c.txHash = txid
	c.batch = batch
	c.chains = map[[32]byte]protocol.Account{}
	c.indices = map[[32]byte]*writeIndex{}
	c.operations = c.operations[:0]
	c.chains = map[[32]byte]protocol.Account{}
	c.indices = map[[32]byte]*writeIndex{}
	_ = c.logger // Get static analsis to shut up
	return c
}

func (c *stateCache) Commit() ([]protocol.Account, error) {
	var create []protocol.Account
	for _, op := range c.operations {
		records, err := op.Execute(c)
		if err != nil {
			return nil, err
		}
		create = append(create, records...)
	}

	return create, nil
}

// LoadUrl loads a chain by URL and unmarshals it.
func (c *stateCache) LoadUrl(account *url.URL) (protocol.Account, error) {
	state, ok := c.chains[account.AccountID32()]
	if ok {
		return state, nil
	}

	state, err := c.batch.Account(account).GetState()
	if err != nil {
		return nil, fmt.Errorf("load %v: get state: %w", account, err)
	}

	c.chains[account.AccountID32()] = state
	return state, nil
}

// LoadUrlAs loads a chain by URL and unmarshals it as a specific type.
func (c *stateCache) LoadUrlAs(account *url.URL, target interface{}) error {
	state, err := c.LoadUrl(account)
	if err != nil {
		return err
	}

	return encoding.SetPtr(state, target)
}

// ReadChain loads an account's chain by URL and name.
func (c *stateCache) ReadChain(u *url.URL, name string) (*database.Chain, error) {
	return c.batch.Account(u).ReadChain(name)
}

//GetHeight loads the height of the chain
func (c *stateCache) GetHeight(u *url.URL) (uint64, error) {
	chain, err := c.batch.Account(u).ReadChain(protocol.MainChain)
	if err != nil {
		return 0, err
	}

	return uint64(chain.Height()), nil
}

// LoadTxn loads and unmarshals a saved transaction
func (c *stateCache) LoadTxn(txid [32]byte) (*protocol.Transaction, error) {
	env, err := c.batch.Transaction(txid[:]).GetState()
	if err != nil {
		return nil, err
	}
	if env.Transaction == nil {
		// This is a signature, not an envelope
		return nil, errors.NotFound("transaction %X not found", txid[:4])
	}
	return env.Transaction, nil
}

func (c *stateCache) AddDirectoryEntry(directory *url.URL, u ...*url.URL) error {
	return AddDirectoryEntry(func(u *url.URL, key ...interface{}) Value {
		return c.RecordIndex(u, key...)
	}, directory, u...)
}

type Value interface {
	Get() ([]byte, error)
	Put([]byte) error
}

func AddDirectoryEntry(getIndex func(*url.URL, ...interface{}) Value, directory *url.URL, u ...*url.URL) error {
	if len(u) == 0 {
		return fmt.Errorf("no URLs supplied to register in directory %s", directory.String())
	}

	mdi := getIndex(directory, "Directory", "Metadata")
	md := new(protocol.DirectoryIndexMetadata)
	data, err := mdi.Get()
	if err == nil {
		err = md.UnmarshalBinary(data)
	}
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("failed to load metadata: %v", err)
	}

	for _, u := range u {
		if !u.Equal(directory) {
			err := getIndex(directory, "Directory", md.Count).Put([]byte(u.String()))
			if err != nil {
				return fmt.Errorf("failed to write index: %v", err)
			}
			md.Count++
		}
	}

	data, err = md.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %v", err)
	}

	return mdi.Put(data)
}

func (st *stateCache) Create(accounts ...protocol.Account) error {
	return st.createOrUpdate(false, accounts)
}

// Update queues a record for storage in the database. The queued update will
// fail if the record does not already exist, unless it is created by a
// synthetic transaction, or the record is a transaction.
func (st *stateCache) Update(accounts ...protocol.Account) error {
	// Update: update an existing record. Non-synthetic transactions are
	// not allowed to create accounts, so we must check if the record
	// already exists. The record may have been added to the DB
	// transaction already, so in order to actually know if the record
	// exists on disk, we have to use GetPersistentEntry.
	return st.createOrUpdate(true, accounts)
}

func (st *stateCache) createOrUpdate(isUpdate bool, accounts []protocol.Account) error {
	isCreate := !isUpdate
	for _, account := range accounts {
		rec := st.batch.Account(account.GetUrl())
		_, err := rec.GetState()
		switch {
		case err != nil && !errors.Is(err, storage.ErrNotFound):
			return fmt.Errorf("failed to check for an existing record: %v", err)

		case err == nil && isCreate:
			return fmt.Errorf("account %v already exists", account.GetUrl())

		case st.txType.IsSynthetic() || st.txType.IsSystem():
			// Synthetic and internal transactions are allowed to create accounts

			// TODO Make synthetic transactions call Create

		case err != nil && isUpdate:
			return fmt.Errorf("account %v does not exist", account.GetUrl())
		}

		if st.Pretend {
			continue
		}

		// Update/Create the state
		err = rec.PutState(account)
		if err != nil {
			return fmt.Errorf("failed to update state of %q: %v", account.GetUrl(), err)
		}

		// Add to the account's main chain
		err = st.state.ChainUpdates.AddChainEntry(st.batch, account.GetUrl(), protocol.MainChain, protocol.ChainTypeTransaction, st.txHash[:], 0, 0)
		if err != nil {
			return fmt.Errorf("failed to update main chain of %q: %v", account.GetUrl(), err)
		}

		// Add it to the directory
		if isCreate {
			u := account.GetUrl()
			err = st.AddDirectoryEntry(u.Identity(), u)
			if err != nil {
				return fmt.Errorf("failed to add a directory entry for %q: %v", u, err)
			}
		}

	}

	return nil
}

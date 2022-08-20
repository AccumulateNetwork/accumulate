package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type stateCache struct {
	*config.Describe
	logger logging.OptionalLogger
	txType protocol.TransactionType
	txHash [32]byte

	Globals    *core.GlobalValues
	State      ProcessTransactionState
	batch      *database.Batch
	operations []stateOperation
	chains     map[[32]byte]protocol.Account

	Pretend bool
}

func newStateCache(net *config.Describe, globals *core.GlobalValues, txtype protocol.TransactionType, txid [32]byte, batch *database.Batch) *stateCache {
	c := new(stateCache)
	c.Describe = net
	c.Globals = globals
	c.txType = txtype
	c.txHash = txid
	c.batch = batch
	c.operations = c.operations[:0]
	c.chains = map[[32]byte]protocol.Account{}
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
		return nil, errors.Format(errors.StatusUnknownError, "load %v: %w", account, err)
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

//GetHeight loads the height of the chain
func (c *stateCache) GetHeight(u *url.URL) (uint64, error) {
	chain, err := c.batch.Account(u).MainChain().Get()
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
	dir := indexing.Directory(c.batch, directory)
	err := dir.Add(u...)
	if err != nil {
		return err
	}
	return nil
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
		if len(account.GetUrl().String()) > protocol.AccountUrlMaxLength {
			return errors.Wrap(errors.StatusBadUrlLength, fmt.Errorf("url specified exceeds maximum character length: %s", account.GetUrl().String()))
		}
		_, err := rec.GetState()
		switch {
		case err != nil && !errors.Is(err, storage.ErrNotFound):
			return errors.Format(errors.StatusUnknownError, "failed to check for an existing record: %v", err)

		case err == nil && isCreate:
			return errors.Format(errors.StatusConflict, "account %v already exists", account.GetUrl())

		case st.txType.IsSynthetic() || st.txType.IsSystem():
			// Synthetic and internal transactions are allowed to create accounts

			// TODO Make synthetic transactions call Create

		case err != nil && isUpdate:
			return errors.Format(errors.StatusConflict, "account %v does not exist", account.GetUrl())
		}

		if st.Pretend {
			continue
		}

		// Update/Create the state
		err = rec.PutState(account)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "failed to update state of %q: %w", account.GetUrl(), err)
		}

		// Add to the account's main chain
		err = st.State.ChainUpdates.AddChainEntry(st.batch, rec.MainChain(), st.txHash[:], 0, 0)
		if err != nil {
			return errors.Format(errors.StatusUnknownError, "failed to update main chain of %q: %w", account.GetUrl(), err)
		}

		// Add it to the directory
		if isCreate {
			u := account.GetUrl()
			err = st.AddDirectoryEntry(u.Identity(), u)
			if err != nil {
				return errors.Format(errors.StatusUnknownError, "failed to add a directory entry for %q: %w", u, err)
			}
		}

	}

	return nil
}

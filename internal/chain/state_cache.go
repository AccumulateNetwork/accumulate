package chain

import (
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

type stateCache struct {
	logger  logging.OptionalLogger
	nodeUrl *url.URL
	txType  protocol.TransactionType
	txHash  types.Bytes32

	blockState BlockState
	batch      *database.Batch
	operations []stateOperation
	chains     map[[32]byte]protocol.Account
	indices    map[[32]byte]*writeIndex
}

func newStateCache(nodeUrl *url.URL, txtype protocol.TransactionType, txid [32]byte, batch *database.Batch) *stateCache {
	c := new(stateCache)
	c.nodeUrl = nodeUrl
	c.txType = txtype
	c.txHash = txid
	c.batch = batch
	c.chains = map[[32]byte]protocol.Account{}
	c.indices = map[[32]byte]*writeIndex{}

	_ = c.logger // Get static analsis to shut up

	c.Reset()
	return c
}

func (c *stateCache) Reset() {
	c.operations = c.operations[:0]
	c.chains = map[[32]byte]protocol.Account{}
	c.indices = map[[32]byte]*writeIndex{}
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
	if env != nil {
		return nil, err
	}
	if env.Transaction == nil {
		// This is a signature, not an envelope
		return nil, fmt.Errorf("transaction %X %w", txid, storage.ErrNotFound)
	}
	return env.Transaction, nil
}

// LoadSignatures loads and unmarshals a transaction's signatures
func (c *stateCache) LoadSignatures(txid [32]byte) (*database.SignatureSet, error) {
	return c.batch.Transaction(txid[:]).GetSignatures()
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

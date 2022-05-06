package database

import (
	"bytes"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func (b *Batch) putBpt(key storage.Key, hash [32]byte) {
	if b.done {
		panic("attempted to use a commited or discarded batch")
	}
	if b.bptEntries == nil {
		panic("attempted to update the BPT after committing the BPT")
	}

	b.bptEntries[key] = hash
}

// CommitBpt updates the Patricia Tree hashes with the values from the updates
// since the last update.
func (b *Batch) CommitBpt() error {
	bpt := pmt.NewBPTManager(b.store)

	for k, v := range b.bptEntries {
		bpt.InsertKV(k, v)
	}

	err := bpt.Bpt.Update()
	if err != nil {
		return err
	}

	b.bptEntries = nil
	return nil
}

func (b *Batch) BptRoot() []byte {
	bpt := pmt.NewBPTManager(b.store)
	return bpt.Bpt.RootHash[:]
}

// BptReceipt builds a BPT receipt for the given key.
func (b *Batch) BptReceipt(key storage.Key, value [32]byte) (*managed.Receipt, error) {
	if len(b.bptEntries) > 0 {
		return nil, errors.New(errors.StatusInternalError, "cannot generate a BPT receipt when there are uncommitted BPT entries")
	}

	bpt := pmt.NewBPTManager(b.store)
	receipt := bpt.Bpt.GetReceipt(key)
	if receipt == nil {
		return nil, errors.NotFound("BPT key %v not found", key)
	}

	return receipt, nil
}

// SaveState writes the full state of the partition out to a file.
func (b *Batch) SaveState(filename string, network *config.Network) error {
	synthetic := object("Account", network.Synthetic())
	subnet := network.NodeUrl()

	bpt := pmt.NewBPTManager(b.store)
	return bpt.Bpt.SaveSnapshot(filename, func(key storage.Key, hash [32]byte) ([]byte, error) {
		// Create an Account object
		account := &Account{b, accountBucket{objectBucket(key)}}

		// Load the main state so we can get the URL
		a, err := account.GetState()
		if err != nil {
			return nil, err
		}

		// Load the full state - preserve chains if the account is a subnet account
		state, err := account.state(true, subnet.PrefixOf(a.GetUrl()))
		if err != nil {
			return nil, err
		}

		hasher, err := state.Hasher()
		if err != nil {
			return nil, err
		}

		if !bytes.Equal(hash[:], hasher.MerkleHash()) {
			return nil, fmt.Errorf("hash does not match for %v\n", state.Main.GetUrl())
		}

		if objectBucket(key) != synthetic {
			return state.MarshalBinary()
		}

		txns, err := account.stateOfTransactionsOnChain(protocol.MainChain)
		if err != nil {
			return nil, err
		}

		for _, txn := range txns {
			if txn.State.Delivered {
				continue
			}

			state.Transactions = append(state.Transactions, txn)
		}

		return state.MarshalBinary()
	})
}

// LoadState loads the full state of the partition from a file.
func (b *Batch) LoadState(filename string) error {
	bpt := pmt.NewBPTManager(b.store)
	return bpt.Bpt.LoadSnapshot(filename, func(key storage.Key, hash [32]byte, reader pmt.SectionReader) error {
		state := new(accountState)
		err := state.UnmarshalBinaryFrom(reader)
		if err != nil {
			return err
		}

		account := &Account{b, accountBucket{objectBucket(key)}}
		err = account.restore(state)
		if err != nil {
			return err
		}

		hasher, err := state.Hasher()
		if err != nil {
			return err
		}

		if !bytes.Equal(hash[:], hasher.MerkleHash()) {
			return fmt.Errorf("hash does not match for %v", state.Main.GetUrl())
		}

		return nil
	})
}

package database

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

// Record manages a record.
type Record struct {
	batch *Batch
	key   recordBucket
}

// GetState loads the record state.
func (r *Record) GetState() (state.Chain, error) {
	data, err := r.batch.store.Get(r.key.State())
	if err != nil {
		return nil, err
	}

	state, err := protocol.UnmarshalChain(data)
	if err != nil {
		return nil, err
	}

	return state, nil
}

// GetStateAs loads the record state and unmarshals into the given value.
func (r *Record) GetStateAs(state state.Chain) error {
	data, err := r.batch.store.Get(r.key.State())
	if err != nil {
		return err
	}

	err = state.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	return nil
}

// PutState stores the record state and adds the record to the BPT (as a hash).
func (r *Record) PutState(recordState state.Chain) error {
	if recordState.Header().ChainUrl == "" {
		return errors.New("invalid URL: empty")
	}
	u, err := recordState.Header().ParseUrl()
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}
	if record(u) != r.key {
		return fmt.Errorf("invalid URL: %v", err)
	}

	switch recordState.(type) {
	case *protocol.LiteTokenAccount, *protocol.LiteDataAccount,
		*protocol.KeyBook, *protocol.KeyPage:
		// Empty key book is OK
	default:
		if recordState.Header().KeyBook == "" {
			return fmt.Errorf("missing key book")
		}
	}

	stateData, err := recordState.MarshalBinary()
	if err != nil {
		return err
	}

	mainChain, err := r.Chain(protocol.Main)
	if err != nil {
		return err
	}

	pendChain, err := r.Chain(protocol.Pending)
	if err != nil {
		return err
	}

	// TODO Create an Object like the old state DB had, with support for
	// multiple chains

	stateHash := sha256.Sum256(stateData)
	data := stateHash[:]
	data = append(data, mainChain.Anchor()...)
	data = append(data, pendChain.Anchor()...)

	r.batch.store.Put(r.key.State(), stateData)
	r.batch.putBpt(r.key.Object(), sha256.Sum256(data))
	return nil
}

// Chain returns a chain manager for the given chain.
func (r *Record) Chain(name string) (*Chain, error) {
	return newChain(r.batch.store, r.key.Chain(name))
}

// Index returns a value that can read or write an index value.
func (r *Record) Index(key ...interface{}) *Value {
	return &Value{r.batch, r.key.Index(key...)}
}

// Data returns a data chain manager for the data chain.
func (r *Record) Data() (*Data, error) {
	chain, err := r.Chain(protocol.Data)
	if err != nil {
		return nil, err
	}

	return &Data{r.batch, r.key, chain}, nil
}

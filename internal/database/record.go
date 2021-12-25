package database

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type Record struct {
	batch *Batch
	key   recordBucket
}

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

func (r *Record) PutState(state state.Chain) error {
	if state.Header().ChainUrl == "" {
		return errors.New("invalid URL: empty")
	}
	u, err := state.Header().ParseUrl()
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}
	if record(u) != r.key {
		return fmt.Errorf("invalid URL: %v", err)
	}

	switch state.Header().Type {
	case types.ChainTypeLiteTokenAccount, types.ChainTypeLiteDataAccount,
		types.ChainTypeKeyBook, types.ChainTypeKeyPage:
	default:
		if state.Header().KeyBook == "" {
			return fmt.Errorf("missing key book")
		}
	}

	stateData, err := state.MarshalBinary()
	if err != nil {
		return err
	}

	chain, err := r.Chain(protocol.Main)
	if err != nil {
		return err
	}

	r.batch.store.Put(r.key.State(), stateData)

	stateHash := sha256.Sum256(stateData)
	hash := sha256.Sum256(append(stateHash[:], chain.Anchor()...))
	r.batch.putBpt(r.key.Object(), hash)
	return nil
}

func (r *Record) Chain(name string) (*Chain, error) {
	return newChain(r.batch.store, r.key.Chain(name))
}

func (r *Record) Index(key ...interface{}) *Value {
	return &Value{r.batch, r.key.Index(key...)}
}

func (r *Record) Data() (*Data, error) {
	chain, err := r.Chain(protocol.Data)
	if err != nil {
		return nil, err
	}

	return &Data{r.batch, r.key, chain}, nil
}

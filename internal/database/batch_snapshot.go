//go:build !example
// +build !example

package database

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

func (b *Batch) ExampleGetFullState(key storage.Key) ([]byte, error) {
	record := &Account{b, accountBucket{objectBucket(key)}}
	obj, err := record.GetObject()
	if err != nil {
		return nil, err
	}
	if obj.Type != protocol.ObjectTypeAccount {
		return nil, fmt.Errorf("object is not an account")
	}

	fullState := new(exampleFullAccountState)
	fullState.Chains = make([]*merkleState, len(obj.Chains))
	fullState.State, err = record.GetState()
	if err != nil {
		return nil, err
	}

	for i, chainMeta := range obj.Chains {
		chain, err := record.ReadChain(chainMeta.Name)
		if err != nil {
			return nil, err
		}

		ms1 := chain.CurrentState()
		ms2 := new(merkleState)
		ms2.Count = uint64(ms1.Count)
		ms2.Pending = make([][32]byte, len(ms1.Pending))
		for i, v := range ms1.Pending {
			if len(v) == 0 {
				continue
			}
			ms2.Pending[i] = v.Bytes32()
		}
		fullState.Chains[i] = ms2
	}

	data, err := fullState.MarshalBinary()
	if err != nil {
		return nil, err
	}

	if len(fullState.Chains) >= 2 {
		test, _ := fullState.Chains[1].MarshalBinary()
		fmt.Println(len(test))
	}

	fmt.Println(len(data))
	return data, nil
}

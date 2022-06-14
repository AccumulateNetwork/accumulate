package database

import (
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type MerkleDbManager struct {
	Batch *Batch
}

func (m MerkleDbManager) State(key storage.Key) managed.DbValue[*managed.MerkleState] {
	return merkleDbState{NewValue(m.Batch, newfn[managed.MerkleState](), key)}
}

func (m MerkleDbManager) Int(key storage.Key) managed.DbInt {
	return merkleDbInt{NewValue(m.Batch, newfn[intValue](), key)}
}
func (m MerkleDbManager) Hash(key storage.Key) managed.DbHash {
	return merkleDbHash{NewValue(m.Batch, newfn[hashValue](), key)}
}

type merkleDbState struct {
	v *ValueAs[*managed.MerkleState]
}

func (m merkleDbState) Get() (*managed.MerkleState, error) {
	v, err := m.v.Get()
	if err != nil {
		return nil, err
	}
	return v.Copy(), nil
}

func (m merkleDbState) Put(v *managed.MerkleState) error {
	return m.v.Put(v.Copy())
}

type merkleDbInt struct {
	v *ValueAs[*intValue]
}

func (m merkleDbInt) Get() (int64, error) {
	v, err := m.v.Get()
	if err != nil {
		return 0, err
	}
	return v.Value, nil
}

func (m merkleDbInt) Put(v int64) error {
	return m.v.Put(&intValue{Value: v})
}

type merkleDbHash struct {
	v *ValueAs[*hashValue]
}

func (m merkleDbHash) Get() (managed.Hash, error) {
	v, err := m.v.Get()
	if err != nil {
		return nil, err
	}
	return v.Value, nil
}

func (m merkleDbHash) Put(v managed.Hash) error {
	return m.v.Put(&hashValue{Value: v})
}

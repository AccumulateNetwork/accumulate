package state

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
)

type Entry interface {
	MarshalBinary() ([]byte, error)
	UnmarshalBinary(data []byte) error
	GetType() *types.Bytes32 //return the Chain Type for the entry.
	GetChainUrl() string
}

type Object struct {
	StateIndex int64       `json:"stateIndex"` //
	Entry      types.Bytes `json:"stateEntry"` //this is the state data that stores the current state of the chain
}

func (app *Object) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	data, err := app.Entry.MarshalBinary()
	if err != nil {
		return nil, err
	}

	var state [8]byte
	n := binary.PutVarint(state[:], app.StateIndex)
	if n <= 0 {
		return nil, fmt.Errorf("unable to marshal state index to a varint")
	}

	buffer.Write(state[:n])
	buffer.Write(data)

	return buffer.Bytes(), nil
}

func (app *Object) UnmarshalBinary(data []byte) error {

	n, i := binary.Varint(data)
	if i <= 0 {
		return fmt.Errorf("insufficient data to unmarshal state index")
	}
	app.StateIndex = n

	if len(data) < i {
		return fmt.Errorf("insufficicient data associated with state entry")
	}

	err := app.Entry.UnmarshalBinary(data[i:])
	if err != nil {
		return fmt.Errorf("no state object associated with state entry, %v", err)
	}

	return nil
}

type StateEntry struct {
	IdentityState *Object
	ChainState    *Object

	//useful cached info
	ChainId  types.Bytes
	AdiChain types.Bytes

	ChainHeader *Chain
	AdiHeader   *Chain

	DB *StateDB
}

func NewStateEntry(idState *Object, chainState *Object, db *StateDB) *StateEntry {
	se := StateEntry{}
	se.IdentityState = idState

	se.ChainState = chainState
	se.DB = db

	return &se
}

const (
	MaskChainState  = 0x01
	MaskAdiState    = 0x02
	MaskChainHeader = 0x04
	MaskAdiHeader   = 0x08
)

func (s *StateEntry) IsValid(mask int) bool {
	if mask&MaskChainState == 1 {
		if s.IdentityState == nil {
			return false
		}
	}

	if mask&MaskAdiState == 1 {
		if s.IdentityState == nil {
			return false
		}
	}

	if mask&MaskChainHeader == 1 {
		if s.ChainHeader == nil {
			return false
		}
	}

	if mask&MaskAdiHeader == 1 {
		if s.AdiHeader == nil {
			return false
		}
	}

	return true
}

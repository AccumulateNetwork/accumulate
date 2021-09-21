package state

import (
	"bytes"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
)

type Entry interface {
	MarshalBinary() ([]byte, error)
	UnmarshalBinary(data []byte) error
	GetType() *types.Bytes32 //return the Chain Type for the entry.
	GetChainUrl() string
}

//maybe we should have Chain header then entry, rather than entry containing all the Headers
type Object struct {
	ChainHeader Chain       `json:"chainHeader"`
	MDRoot      types.Bytes `json:"pendingMDRoot"`
	//BPTRoot       types.Bytes `json:"bptRoot"`
	Entry types.Bytes `json:"stateEntry"` //this is the state data that stores the current state of the chain
}

func (app *Object) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	data, err := app.ChainHeader.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(data)

	data, err = app.Entry.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(data)

	return buffer.Bytes(), nil
}

func (app *Object) UnmarshalBinary(data []byte) error {
	//minimum length of a chain header is 33 bytes
	if len(data) < 33 {
		return fmt.Errorf("insufficicient data associated with state entry")
	}

	err := app.ChainHeader.UnmarshalBinary(data)
	if err != nil {
		return fmt.Errorf("cannot unmarshal chain header associated with state, %v", err)
	}
	i := app.ChainHeader.GetHeaderSize()

	err = app.Entry.UnmarshalBinary(data[i:])
	if err != nil {
		return fmt.Errorf("no state object associated with state entry, %v", err)
	}

	return nil
}

type StateEntry struct {
	IdentityState *Object
	ChainState    *Object

	//useful cached info
	ChainId  *types.Bytes32
	AdiChain *types.Bytes32

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

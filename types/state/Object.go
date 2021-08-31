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

type Object struct {
	Chain
	StateHash     types.Bytes `json:"stateHash"`     //this is the same as the entry hash.
	PrevStateHash types.Bytes `json:"prevStateHash"` //not sure if we need this since we are only keeping up with current state
	EntryHash     types.Bytes `json:"entryHash"`     //not sure if this is needed since it is baked into state hash...
	Entry         types.Bytes `json:"entry"`         //this is the state data that stores the current state of the chain
}

func (app *Object) Marshal() ([]byte, error) {
	var buffer bytes.Buffer

	data, err := app.Chain.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(data)

	data, err = app.StateHash.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(data)

	data, err = app.PrevStateHash.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(data)

	data, err = app.EntryHash.MarshalBinary()
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

func (app *Object) Unmarshal(data []byte) error {
	err := app.Chain.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	i := app.GetHeaderSize()

	if len(data) < i {
		return fmt.Errorf("invalid data before state hash")
	}

	err = app.StateHash.UnmarshalBinary(data[i:])
	if err != nil {
		return err
	}

	i += app.StateHash.Size(nil)

	if len(data) < i {
		return fmt.Errorf("invalid data before previous state hash")
	}

	err = app.PrevStateHash.UnmarshalBinary(data[i:])
	if err != nil {
		return err
	}

	i += app.PrevStateHash.Size(nil)

	if len(data) < i {
		return fmt.Errorf("invalid data before entry hash")
	}

	err = app.EntryHash.UnmarshalBinary(data[i:])
	if err != nil {
		return err
	}

	i += app.EntryHash.Size(nil)

	if len(data) < i {
		return fmt.Errorf("invalid data before entry")
	}

	err = app.Entry.UnmarshalBinary(data[i:])
	if err != nil {
		return err
	}

	return nil
}

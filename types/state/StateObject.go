package state

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"github.com/AccumulateNetwork/SMT/managed"
)

type StateEntry interface {
	//MarshalBinary() ([]byte, error)
	//UnmarshalBinary(data []byte) error
	//MarshalJSON() (string, error)
	//UnmarshalJSON(s string) error
	Type() string //return the Chain Type for the entry.
}

type StateObject struct {
	Type          string
	StateHash     []byte //this is the same as the entry hash.
	PrevStateHash []byte //not sure if we need this since we are only keeping up with current state
	EntryHash     []byte //not sure this is needed since it is baked into state hash...
	Entry         []byte //this is the data that stores the current state of the chain
}

func (app *StateObject) Marshal() ([]byte, error) {
	var ret []byte

	if len(app.Type) == 0 {
		return nil, fmt.Errorf("State Object type not specified")
	}

	ret = append(ret, byte(len(app.Type)))
	ret = append(ret, app.Type...)
	ret = append(ret, app.StateHash...)
	ret = append(ret, app.PrevStateHash...)
	ret = append(ret, app.EntryHash...)
	ret = append(ret, app.Entry...)

	return ret, nil
}

func (app *StateObject) Unmarshal(data []byte) error {
	if len(data) < 1+32+32+32+1 {
		return fmt.Errorf("Insufficient data to unmarshall State Entry.")
	}

	if len(data)-int(data[0]) < 1+32+32+32+1 {
		return fmt.Errorf("Insufficient data for on State object for state type")
	}
	app.StateHash = managed.Hash{}.Bytes()
	app.PrevStateHash = managed.Hash{}.Bytes()
	app.EntryHash = managed.Hash{}.Bytes()

	app.Type = string(data[1 : 1+data[0]])
	i := int(data[0]) + 1
	i += copy(app.StateHash, data[i:i+32])
	i += copy(app.PrevStateHash, data[i:i+32])
	i += copy(app.EntryHash, data[i:i+i+32])
	entryhash := sha256.Sum256(data[i:])
	if bytes.Compare(app.EntryHash, entryhash[:]) != 0 {
		return fmt.Errorf("Entry Hash does not match the data hash")
	}
	app.Entry = data[i:]

	return nil
}

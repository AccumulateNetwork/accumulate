package state

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"github.com/AccumulateNetwork/SMT/managed"
)

type StateEntry interface {
	MarshalBinary() ([]byte, error)
	UnmarshalBinary(data []byte) error
}

type StateObject struct {
	StateHash     []byte //this is the same as the entry hash.
	PrevStateHash []byte //not sure if we need this since we are only keeping up with current state
	EntryHash     []byte //not sure this is needed since it is baked into state hash...
	Entry         []byte //this is the data that stores the current state of the chain
}

func (app *StateObject) Marshal() ([]byte, error) {
	var ret []byte

	ret = append(ret, app.StateHash...)
	ret = append(ret, app.PrevStateHash...)
	ret = append(ret, app.EntryHash...)
	ret = append(ret, app.Entry...)

	return ret, nil
}

func (app *StateObject) Unmarshal(data []byte) error {
	if len(data) < 32+32+32+1 {
		return fmt.Errorf("Insufficient data to unmarshall State Entry.")
	}

	app.StateHash = managed.Hash{}.Bytes()
	app.PrevStateHash = managed.Hash{}.Bytes()
	app.EntryHash = managed.Hash{}.Bytes()

	i := 0
	i += copy(app.StateHash, data[i:32])
	i += copy(app.PrevStateHash, data[i:32])
	i += copy(app.EntryHash, data[i:i+32])
	entryhash := sha256.Sum256(data[i:])
	if bytes.Compare(app.EntryHash, entryhash[:]) != 0 {
		return fmt.Errorf("Entry Hash does not match the data hash")
	}
	app.Entry = data[i:]

	return nil
}

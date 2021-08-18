package state

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
)

type Entry interface {
	MarshalBinary() ([]byte, error)
	UnmarshalBinary(data []byte) error
	MarshalJSON() ([]byte, error)
	UnmarshalJSON(s []byte) error
	GetType() string //return the Chain Type for the entry.
	GetAdiChainPath() string
}

type Object struct {
	Header
	StateHash     types.Bytes `json:"state-hash"`      //this is the same as the entry hash.
	PrevStateHash types.Bytes `json:"prev-state-hash"` //not sure if we need this since we are only keeping up with current state
	EntryHash     types.Bytes `json:"entry-hash"`      //not sure if this is needed since it is baked into state hash...
	Entry         types.Bytes `json:"entry"`           //this is the state data that stores the current state of the chain
}

func (app *Object) Marshal() ([]byte, error) {
	var ret []byte

	if len(app.Type) == 0 {
		return nil, fmt.Errorf("state object type not specified")
	}

	ret = append(ret, byte(len(app.Type)))
	ret = append(ret, app.Type...)
	ret = append(ret, app.StateHash...)
	ret = append(ret, app.PrevStateHash...)
	ret = append(ret, app.EntryHash...)
	ret = append(ret, app.Entry...)

	return ret, nil
}

func (app *Object) Unmarshal(data []byte) error {
	if len(data) < 1+32+32+32+1 {
		return fmt.Errorf("insufficient data to unmarshall State Entry")
	}

	if len(data)-int(data[0]) < 1+32+32+32+1 {
		return fmt.Errorf("insufficient data for on State object for state type")
	}
	app.StateHash = types.Bytes32{}.Bytes()
	app.PrevStateHash = types.Bytes32{}.Bytes()
	app.EntryHash = types.Bytes32{}.Bytes()

	app.Type = types.String(data[1 : 1+data[0]])
	i := int(data[0]) + 1
	i += copy(app.StateHash, data[i:i+32])
	i += copy(app.PrevStateHash, data[i:i+32])
	i += copy(app.EntryHash, data[i:i+i+32])
	entryhash := sha256.Sum256(data[i:])
	if bytes.Compare(app.EntryHash, entryhash[:]) != 0 {
		return fmt.Errorf("entry Hash does not match the data hash")
	}

	app.Entry = data[i:]

	return nil
}

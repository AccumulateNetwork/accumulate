package state

import (
	"bytes"
	"encoding"
	"errors"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/smt/common"

	"github.com/AccumulateNetwork/accumulated/types"
)

//maybe we should have Chain header then entry, rather than entry containing all the Headers
type Object struct {
	ChainHeader Chain       `json:"chainHeader"`
	MDRoot      types.Bytes `json:"pendingMDRoot"`
	Entry       types.Bytes `json:"stateEntry"` //this is the state data that stores the current state of the chain
}

func (app *Object) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	data, err := app.ChainHeader.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(data)

	buffer.Write(common.SliceBytes(app.MDRoot.Bytes()))

	data, err = app.Entry.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(data)

	return buffer.Bytes(), nil
}

func (app *Object) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if rErr := recover(); rErr != nil {
			err = fmt.Errorf("error unmarshaling Chain State Object, %v", rErr)
		}
	}()

	//minimum length of a chain header is 33 bytes
	if len(data) < 33 {
		return fmt.Errorf("insufficicient data associated with state entry")
	}

	err = app.ChainHeader.UnmarshalBinary(data)
	if err != nil {
		return fmt.Errorf("cannot unmarshal chain header associated with state, %v", err)
	}
	i := app.ChainHeader.GetHeaderSize()

	app.MDRoot, data = common.BytesSlice(data[i:])

	err = app.Entry.UnmarshalBinary(data)
	if err != nil {
		return fmt.Errorf("no state object associated with state entry, %v", err)
	}

	return nil
}

func (o *Object) As(entry encoding.BinaryUnmarshaler) error {
	return entry.UnmarshalBinary(o.Entry)
}

type StateEntry struct {
	AdiState   *Object
	ChainState *Object

	//useful cached info
	ChainId  *types.Bytes32
	AdiChain *types.Bytes32

	ChainHeader *Chain
	AdiHeader   *Chain

	DB *StateDB
}

// LoadChainAndADI retrieves the specified chain and unmarshals it, and
// retrieves its ADI and unmarshals it.
func (db *StateDB) LoadChainAndADI(chainId []byte) (*StateEntry, error) {
	chainState, chainHeader, err := db.LoadChain(chainId)
	if errors.Is(err, ErrNotFound) {
		return &StateEntry{
			DB: db,
		}, nil
	} else if err != nil {
		return nil, err
	}

	adiChain, adiState, adiHeader, err := db.LoadChainADI(chainHeader)
	if errors.Is(err, ErrNotFound) {
		return &StateEntry{
			DB:          db,
			ChainState:  chainState,
			ChainHeader: chainHeader,
			AdiChain:    adiChain,
		}, nil
	} else if err != nil {
		return nil, err
	}

	return &StateEntry{
		DB:          db,
		ChainState:  chainState,
		ChainHeader: chainHeader,
		AdiChain:    adiChain,
		AdiState:    adiState,
		AdiHeader:   adiHeader,
	}, nil
}

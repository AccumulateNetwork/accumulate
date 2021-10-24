package state

import (
	"bytes"
	"encoding"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
)

//maybe we should have Chain header then entry, rather than entry containing all the Headers
type Object struct {
	MDRoot types.Bytes32 `json:"mdRoot"`
	Entry  types.Bytes   `json:"state"` //this is the state data that stores the current state of the chain
}

func (app *Object) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(app.MDRoot.Bytes())

	data, err := app.Entry.MarshalBinary()
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

	app.MDRoot.FromBytes(data)

	err = app.Entry.UnmarshalBinary(data[32:])
	if err != nil {
		return fmt.Errorf("no state object associated with state entry, %v", err)
	}

	return nil
}

func (o *Object) As(entry encoding.BinaryUnmarshaler) error {
	return entry.UnmarshalBinary(o.Entry)
}

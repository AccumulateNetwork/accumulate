package state

import (
	"bytes"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/types"
)

type DataAccount struct {
	ChainHeader
	ManagerKeyBookUrl types.String `json:"managerKeyBookUrl"` //this probably should be moved to chain header
}

//NewDataAccount create a new data account.
//Requires the chain id and data account url and manager key book url if applicable
func NewDataAccount(accountUrl string, managerKeyBookUrl string) *DataAccount {
	tas := DataAccount{}

	tas.SetHeader(types.String(accountUrl), types.ChainTypeDataAccount)
	tas.ManagerKeyBookUrl = types.String(managerKeyBookUrl)

	return &tas
}

//MarshalBinary creates a byte array of the state object needed for storage
func (app *DataAccount) MarshalBinary() (ret []byte, err error) {
	var buffer bytes.Buffer

	header, err := app.ChainHeader.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(header)

	managerKeyBookUrlData, err := app.ManagerKeyBookUrl.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("cannot marshal binary for token URL in DataAccount, %v", err)
	}
	buffer.Write(managerKeyBookUrlData)

	return buffer.Bytes(), nil
}

//UnmarshalBinary will deserialize a byte array
func (app *DataAccount) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error marshaling TokenTx State %v", r)
		}
	}()

	err = app.ChainHeader.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	if app.Type != types.ChainTypeDataAccount {
		return fmt.Errorf("invalid chain type: want %v, got %v", types.ChainTypeTokenAccount, app.Type)
	}

	i := app.GetHeaderSize()

	err = app.ManagerKeyBookUrl.UnmarshalBinary(data[i:])
	if err != nil {
		return fmt.Errorf("unable to unmarshal binary for token account, %v", err)
	}

	i += app.ManagerKeyBookUrl.Size(nil)

	return nil
}

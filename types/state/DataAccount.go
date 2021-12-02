package state

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/smt/managed"
	"github.com/AccumulateNetwork/accumulate/types"
)

type DataAccount struct {
	ChainHeader
	ManagerKeyBookUrl types.String        `json:"managerKeyBookUrl"` //this probably should be moved to chain header
	EntrySMT          managed.MerkleState `json:"entryState"`        //store the merkle state of the entry hashes
}

//NewTokenAccount create a new token account.  Requires the identity/chain id's and coinbase if applicable
func NewDataAccount(accountUrl string, managerKeyBookUrl string) *DataAccount {
	tas := DataAccount{}

	tas.SetHeader(types.String(accountUrl), types.ChainTypeDataAccount)
	tas.ManagerKeyBookUrl = types.String(managerKeyBookUrl)

	return &tas
}

// ComputeEntryHash returns the entry hash given the chainid, external id's, and data
func ComputeEntryHash(chainId []byte, extIds [][]byte, data []byte) types.Bytes {
	smt := managed.MerkleState{}
	//Seed the smt with the chainId
	smt.AddToMerkleTree(chainId)
	//add the external id's to the merkle tree
	for i := range extIds {
		h := sha256.Sum256(extIds[i])
		smt.AddToMerkleTree(h[:])
	}
	//add the data to the merkle tree
	h := sha256.Sum256(data)
	smt.AddToMerkleTree(h[:])
	//return the entry hash
	return smt.GetMDRoot().Bytes()
}

// UpdateMerkleState updates the state of the data account given a new entry hash.
func (app *DataAccount) UpdateMerkleState(chainId []byte, extIds [][]byte, data []byte) {
	//Build a merkle state to compute the entry hash.
	app.EntrySMT.AddToMerkleTree(ComputeEntryHash(chainId, extIds, data))
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

	smt, err := app.EntrySMT.Marshal()
	if err != nil {
		return nil, fmt.Errorf("cannot marshal binary for SMT in DataAccount, %v", err)
	}
	buffer.Write(smt)

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

	err = app.EntrySMT.UnMarshal(data[i:])
	if err != nil {
		return err
	}

	return nil
}

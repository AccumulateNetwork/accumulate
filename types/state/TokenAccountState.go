package state

import (
	"fmt"
	"github.com/AccumulateNetwork/SMT/managed"
	"math/big"
)

type TokenAccountState struct {
	StateEntry
	issueridentity managed.Hash //need to know who issued tokens
	issuerchainid  managed.Hash //identity/issue chains both hold the metrics for the TokenRules ... hmm.. do those need to be passed along since those need to be used
	balance        big.Int
}

func NewTokenAccountState(issuerid []byte, issuerchain []byte) *TokenAccountState {
	tas := TokenAccountState{}
	copy(tas.issueridentity[:], issuerid)
	copy(tas.issuerchainid[:], issuerchain)
	return &tas
}

const TokenAccountStateLen = 32 + 32 + 32

func (ts *TokenAccountState) GetIssuerIdentity() *managed.Hash {
	return &ts.issueridentity
}

func (ts *TokenAccountState) GetIssuerChainId() *managed.Hash {
	return &ts.issuerchainid
}

func (ts *TokenAccountState) SubBalance(amt *big.Int) error {

	if amt == nil {
		return fmt.Errorf("Invalid input amount specified to subtract from balance")
	}

	if ts.balance.Cmp(amt) < 0 {
		return fmt.Errorf("{ \"Insufficient-Balance\" : { \"Available\" : \"%d\" , \"Requested\", \"%d\" } }", ts.Balance(), amt)
	}

	ts.balance.Sub(&ts.balance, amt)
	return nil
}

func (ts *TokenAccountState) Balance() *big.Int {
	return &ts.balance
}

func (ts *TokenAccountState) AddBalance(amt *big.Int) error {

	if amt == nil {
		return fmt.Errorf("Invalid input amount specified to add to balance")
	}

	ts.balance.Add(&ts.balance, amt)
	return nil
}

func (ts *TokenAccountState) MarshalBinary() ([]byte, error) {

	data := make([]byte, TokenAccountStateLen)

	i := copy(data[:], ts.issueridentity.Bytes())
	i += copy(data[i:], ts.issuerchainid.Bytes())

	ts.balance.FillBytes(data[i:])

	return data, nil
}

func (ts *TokenAccountState) UnmarshalBinary(data []byte) error {

	if len(data) < TokenAccountStateLen {
		return fmt.Errorf("Invalid Token Data for unmarshalling %X on chain %X", ts.issueridentity, ts.issuerchainid)
	}

	i := copy(ts.issueridentity.Bytes(), data[:])
	i += copy(ts.issuerchainid.Bytes(), data[i:])

	ts.balance.SetBytes(data[i:])
	return nil
}

//func (app *TokenAccountState) MarshalEntry(chainid *managed.Hash) (*Entry, error) {
//	e := Entry{}
//	e.ChainID = chainid
//	data := make([]byte,8)
//	binary.BigEndian.PutUint64(data, app.balance)
//	//Token balance is maintained in external id.
//	e.ExtIDs = make([][]byte,1)
//	return nil, nil
//}
//
//func (app *TokenAccountState) UnmarshalEntry(entry *Entry) error {
//	//i := 1
//	//i += copy(data[i:], e.ChainID[:])
//	//binary.BigEndian.PutUint16(data[i:i+2],
//	//	uint16(totalSize-len(e.Content)-EntryHeaderSize))
//	return nil
//}
//

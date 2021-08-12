package state

import (
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"math/big"
)

type TokenAccountState struct {
	StateEntry
	issueridentity types.Bytes32 //need to know who issued tokens
	issuerchainid  types.Bytes32 //identity/issue chains both hold the metrics for the TokenRules ... hmm.. do those need to be passed along since those need to be used
	balance        big.Int
	coinbase       *types.TokenIssuance
}

func NewTokenAccountState(issuerid []byte, issuerchain []byte, coinbase *types.TokenIssuance) *TokenAccountState {
	tas := TokenAccountState{}
	copy(tas.issueridentity[:], issuerid)
	copy(tas.issuerchainid[:], issuerchain)
	tas.coinbase = coinbase
	return &tas
}

const tokenAccountStateLen = 32 + 32 + 32

func (ts *TokenAccountState) Type() string {
	return "AIM-0"
}

func (ts *TokenAccountState) GetIssuerIdentity() *types.Bytes32 {
	return &ts.issueridentity
}

func (ts *TokenAccountState) GetIssuerChainId() *types.Bytes32 {
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

func (ts *TokenAccountState) MarshalBinary() (ret []byte, err error) {

	var coinbase []byte
	if ts.coinbase != nil {
		coinbase, err = ts.coinbase.MarshalBinary()
		if err != nil {
			return nil, err
		}
	}
	data := make([]byte, tokenAccountStateLen+len(coinbase))

	i := copy(data[:], ts.issueridentity[:])
	i += copy(data[i:], ts.issuerchainid[:])

	ts.balance.FillBytes(data[i:])
	i += 32

	if ts.coinbase != nil {
		copy(data[i:], coinbase)
	}

	return data, nil
}

func (ts *TokenAccountState) UnmarshalBinary(data []byte) error {

	if len(data) < tokenAccountStateLen {
		return fmt.Errorf("Invalid Token Data for unmarshalling %X on chain %X", ts.issueridentity, ts.issuerchainid)
	}

	i := copy(ts.issueridentity[:], data[:])
	i += copy(ts.issuerchainid[:], data[i:])

	ts.balance.SetBytes(data[i:])
	i += 32

	if len(data) > i {
		coinbase := types.TokenIssuance{}
		err := coinbase.UnmarshalBinary(data[i:])
		if err != nil {
			return err
		}
		ts.coinbase = &coinbase
	}

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

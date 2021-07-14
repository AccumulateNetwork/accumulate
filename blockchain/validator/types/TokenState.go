package types

import (
	"fmt"
	"github.com/AccumulateNetwork/SMT/managed"
	"github.com/shopspring/decimal"
)

type TokenState struct {
	StateEntry
	issueridentity managed.Hash
	issuertokenchainid managed.Hash
	balance decimal.Decimal
	//balance big.Int
}
//
//{
//"type": "FAT-0",
//"supply": 10000000,
//"precision": 5,
//"symbol": "EXT",
//"metadata": {"custom-field": "example"}
//}
//this is part of the token chain
type TokenRules struct {
    tokentype string //
    supply uint64
    precision int8
    symbol string
    metadata string //don't need here
}

const TokenStateLen = 32+32

func (ts *TokenState) IssuerIdentity() *managed.Hash {
	return &ts.issueridentity
}

func (ts *TokenState) IssuerTokenChainId() *managed.Hash {
	return &ts.issuertokenchainid

}

func (ts *TokenState) Deposit(amt string) error {
	inputamt, err := decimal.NewFromString(amt)

	if err != nil {
		return err
	}

	ts.balance = ts.balance.Add(inputamt)
    return nil
}

func (ts *TokenState) Balance() string {
	return ts.balance.StringFixedBank(2)
}

func (ts *TokenState) Withdrawal(amt string, fee string) error {

	outputamt, err := decimal.NewFromString(amt)

	if err != nil {
		return err
	}

	feeamt, err := decimal.NewFromString(fee)

	if err != nil {
		return err
	}

	total := outputamt.Add(feeamt)

	if ts.balance.Cmp(total) < 0 {
		return fmt.Errorf("Insufficient Balance")
	}

	ts.balance = ts.balance.Sub(total)

	return nil
}

func (ts *TokenState) MarshalBinary() ([]byte, error) {
	bal := []byte(ts.Balance())
	data := make([]byte, TokenStateLen + len(bal))
	i := copy(data[:], ts.issueridentity.Bytes())
	i += copy(data[i:], ts.issueridentity.Bytes())
	copy(data[i:], bal)

	return data, nil
}

func (ts *TokenState) UnmarshalBinary(data []byte) error {

	if len(data) !=  TokenStateLen {
		return fmt.Errorf("Invalid Token Data for unmarshalling %X on chain %X", ts.issueridentity, ts.issuertokenchainid)
	}
	i := copy(ts.issueridentity.Bytes(), data[:])
	i += copy(ts.issuertokenchainid.Bytes(), data[i:])

	var err error
	ts.balance, err = decimal.NewFromString(string(data[i:]))

	return err
}

//func (app *TokenState) MarshalEntry(chainid *managed.Hash) (*Entry, error) {
//	e := Entry{}
//	e.ChainID = chainid
//	data := make([]byte,8)
//	binary.BigEndian.PutUint64(data, app.balance)
//	//Token balance is maintained in external id.
//	e.ExtIDs = make([][]byte,1)
//	return nil, nil
//}
//
//func (app *TokenState) UnmarshalEntry(entry *Entry) error {
//	//i := 1
//	//i += copy(data[i:], e.ChainID[:])
//	//binary.BigEndian.PutUint16(data[i:i+2],
//	//	uint16(totalSize-len(e.Content)-EntryHeaderSize))
//	return nil
//}
//

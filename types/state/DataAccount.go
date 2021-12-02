package state

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/types"
)

type DataAccount struct {
	ChainHeader
	ManagerKeyBookUrl types.String    `json:"managerKeyBookUrl"`
	EntryHashes       []types.Bytes32 `json:"entries"` //need to know who issued tokens, this can be condensed maybe back to adi chain path
}

//NewTokenAccount create a new token account.  Requires the identity/chain id's and coinbase if applicable
func NewDataAccount(accountUrl string, managerKeyBookUrl string) *DataAccount {
	tas := DataAccount{}

	tas.SetHeader(types.String(accountUrl), types.ChainTypeDataAccount)
	tas.ManagerKeyBookUrl = types.String(managerKeyBookUrl)

	return &tas
}

//Set will copy a token account state
func (app *TokenAccount) Set(accountState *TokenAccount) {
	if accountState == nil {
		return
	}
	app.Balance.Set(&accountState.Balance)
	app.ChainUrl = accountState.ChainUrl
	app.TokenUrl = accountState.TokenUrl
	app.Type = accountState.Type
}

// CanTransact returns true/false if there is a sufficient balance
func (app *TokenAccount) ComputeEntryHash(extIds []byte, data []byte) bool {
	//Build a merkle tree to compute the entry hash.

	return app.GetBalance().Cmp(amt) >= 0
}

//SubBalance will subtract a balance form the account.  If this is a coinbase account,
//the balance will not be subtracted.
func (app *TokenAccount) SubBalance(amt *big.Int) error {
	if amt == nil {
		return fmt.Errorf("invalid input amount specified to subtract from balance")
	}

	if app.Balance.Cmp(amt) < 0 {
		return fmt.Errorf("insufficient balance, amount available : %d, requested, %d", app.GetBalance(), amt)
	}

	app.Balance.Sub(&app.Balance, amt)
	return nil
}

//GetBalance will return the balance of the account
func (app *TokenAccount) GetBalance() *big.Int {
	return &app.Balance
}

//AddBalance will add an amount to the balance. It only accepts a positive balance.
func (app *TokenAccount) AddBalance(amt *big.Int) error {
	if amt == nil {
		return fmt.Errorf("invalid input amount specified to add to balance")
	}

	if amt.Sign() <= 0 {
		return fmt.Errorf("amount to add to balance must be a positive amount")
	}

	app.Balance.Add(&app.Balance, amt)
	return nil
}

//MarshalBinary creates a byte array of the state object needed for storage
func (app *TokenAccount) MarshalBinary() (ret []byte, err error) {
	var buffer bytes.Buffer

	header, err := app.ChainHeader.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buffer.Write(header)

	tokenUrlData, err := app.TokenUrl.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("cannot marshal binary for token URL in TokenAccount, %v", err)
	}
	buffer.Write(tokenUrlData)

	buffer.Write(common.SliceBytes(app.Balance.Bytes()))
	buffer.Write(common.Uint64Bytes(app.TxCount))

	return buffer.Bytes(), nil
}

//UnmarshalBinary will deserialize a byte array
func (app *TokenAccount) UnmarshalBinary(data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("error marshaling TokenTx State %v", r)
		}
	}()

	err = app.ChainHeader.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	if app.Type != types.ChainTypeTokenAccount {
		return fmt.Errorf("invalid chain type: want %v, got %v", types.ChainTypeTokenAccount, app.Type)
	}

	i := app.GetHeaderSize()

	err = app.TokenUrl.UnmarshalBinary(data[i:])
	if err != nil {
		return fmt.Errorf("unable to unmarshal binary for token account, %v", err)
	}

	i += app.TokenUrl.Size(nil)

	bal, data := common.BytesSlice(data[i:])

	app.Balance.SetBytes(bal)
	app.TxCount, _ = common.BytesUint64(data)

	return nil
}

func (acct *TokenAccount) CreditTokens(amount *big.Int) bool {
	if amount == nil || amount.Sign() < 0 {
		return false
	}

	acct.Balance.Add(&acct.Balance, amount)
	return true
}

func (acct *TokenAccount) CanDebitTokens(amount *big.Int) bool {
	return amount != nil && acct.Balance.Cmp(amount) >= 0
}

func (acct *TokenAccount) DebitTokens(amount *big.Int) bool {
	if !acct.CanDebitTokens(amount) {
		return false
	}

	acct.Balance.Sub(&acct.Balance, amount)
	return true
}

func (acct *TokenAccount) NextTx() uint64 {
	c := acct.TxCount
	acct.TxCount++
	return c
}

func (acct *TokenAccount) ParseTokenUrl() (*url.URL, error) {
	return url.Parse(*acct.TokenUrl.AsString())
}

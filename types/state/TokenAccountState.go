package state

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"math/big"
)

type tokenAccountState struct {
	Header
	TokenUrl types.String `json:"tokenUrl"`           //need to know who issued tokens, this can be condensed maybe back to adi chain path
	Balance  big.Int      `json:"balance"`            //store the balance as a big int.
	Coinbase *types.Token `json:"coinbase,omitempty"` //this is nil for a non-coinbase account, has token info otherwise
}

type TokenAccountState struct {
	Entry
	tokenAccountState
}

//NewTokenAccountState create a new token account.  Requires the identity/chain id's and coinbase if applicable
func NewTokenAccountState(accountUrl types.UrlChain, tokenUrl types.UrlChain, coinbase *types.Token) *TokenAccountState {
	tas := TokenAccountState{}
	tas.Type = sha256.Sum256([]byte("AIM/1/0.1"))
	tas.AdiChainPath = types.String(accountUrl)
	tas.Coinbase = coinbase
	if tas.IsCoinbaseAccount() {
		tas.TokenUrl = coinbase.URL
	}

	return &tas
}

//Set will copy a token account state
func (app *TokenAccountState) Set(accountState *TokenAccountState) {
	if accountState == nil {
		return
	}
	app.Coinbase = accountState.Coinbase
	app.Balance.Set(&accountState.Balance)
	app.AdiChainPath = accountState.AdiChainPath
	app.TokenUrl = accountState.TokenUrl
	app.Type = accountState.Type
}

// GetType is an implemented interface that returns the chain type of the object
func (app *TokenAccountState) GetType() *types.Bytes32 {
	return app.GetType()
}

//GetAdiChainPath returns the chain path for the object in the chain.
func (app *TokenAccountState) GetAdiChainPath() string {
	return string(app.AdiChainPath)
}

//SubBalance will subtract a balance form the account.  If this is a coinbase account,
//the balance will not be subtracted.
func (app *TokenAccountState) SubBalance(amt *big.Int) error {
	if app.IsCoinbaseAccount() {
		//if we are a coinbase account, never subtract balance
		return nil
	}

	if amt == nil {
		return fmt.Errorf("invalid input amount specified to subtract from balance")
	}

	if app.Balance.Cmp(amt) < 0 {
		return fmt.Errorf("{ \"insufficient-balance\" : { \"available\" : \"%d\" , \"requested\", \"%d\" } }", app.GetBalance(), amt)
	}

	app.Balance.Sub(&app.Balance, amt)
	return nil
}

//GetBalance will return the balance of the account
func (app *TokenAccountState) GetBalance() *big.Int {
	return &app.Balance
}

//AddBalance will add an amount to the balance. It only accepts a positive balance.
func (app *TokenAccountState) AddBalance(amt *big.Int) error {
	//if this a coinbase account, do nothing
	if app.Coinbase != nil {
		//check to see if we are a coinbase, and if so, no funds should be added to
		//this account so return with no error
		return nil
	}

	if amt == nil {
		return fmt.Errorf("invalid input amount specified to add to balance")
	}

	if amt.Sign() <= 0 {
		return fmt.Errorf("amount to add to balance must be a positive amount")
	}

	app.Balance.Add(&app.Balance, amt)
	return nil
}

//IsCoinbaseAccount will return true if this is a coinbase, false otherwise
func (app *TokenAccountState) IsCoinbaseAccount() bool {
	return app.Coinbase != nil
}

//MarshalBinary creates a byte array of the state object needed for storage
func (app *TokenAccountState) MarshalBinary() (ret []byte, err error) {
	var coinbase []byte
	if app.Coinbase != nil {
		coinbase, err = app.Coinbase.MarshalBinary()
		if err != nil {
			return nil, err
		}
	}

	header, err := app.Header.MarshalBinary()
	if err != nil {
		return nil, err
	}

	data := make([]byte, len(header)+32+len(coinbase))

	i := copy(data, header)

	app.Balance.FillBytes(data[i:])
	i += 32

	if app.Coinbase != nil {
		copy(data[i:], coinbase)
	}

	return data, nil
}

//UnmarshalBinary will deserialize a byte array
func (app *TokenAccountState) UnmarshalBinary(data []byte) error {
	err := app.Header.UnmarshalBinary(data)
	if err != nil {
		return err
	}

	i := app.GetHeaderSize()

	if len(data) < i+32 {
		return fmt.Errorf("invalid data buffer to unmarshal account state")
	}

	app.Balance.SetBytes(data[i : i+32])
	i += 32

	if len(data) > i {
		coinbase := types.Token{}
		err := coinbase.UnmarshalBinary(data[i:])
		if err != nil {
			return err
		}
		app.Coinbase = &coinbase
	}

	return nil
}

//UnmarshalJSON will convert a json string into the Token Account
func (app *TokenAccountState) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &app.tokenAccountState)
}

//MarshalJSON will convert the Token Account into a json string
func (app *TokenAccountState) MarshalJSON() ([]byte, error) {
	return json.Marshal(&app.tokenAccountState)
}

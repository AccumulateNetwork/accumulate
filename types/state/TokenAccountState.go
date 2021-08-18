package state

import (
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"math/big"
)

type tokenAccountState struct {
	Header
	IssuerIdentity types.Bytes32        `json:"issuer-identity"`    //need to know who issued tokens, this can be condensed maybe back to adi chain path
	IssuerChainId  types.Bytes32        `json:"issuer-chain-id"`    //identity/issue chains both hold the metrics for the TokenRules
	Balance        big.Int              `json:"balance"`            //store the balance as a big int.
	Coinbase       *types.TokenIssuance `json:"coinbase,omitempty"` //this is nil for a non-coinbase account, has token info otherwise
}

type TokenAccountState struct {
	Entry
	tokenAccountState
}

//NewTokenAccountState create a new token account.  Requires the identity/chain id's and coinbase if applicable
func NewTokenAccountState(issuerid []byte, issuerchain []byte, coinbase *types.TokenIssuance) *TokenAccountState {
	tas := TokenAccountState{}
	tas.Type = "AIM-0"
	tas.AdiChainPath = "unknown"
	copy(tas.IssuerIdentity[:], issuerid)
	copy(tas.IssuerChainId[:], issuerchain)
	tas.Coinbase = coinbase

	//set the initial supply if a coinbase account
	if tas.IsCoinbaseAccount() {
		if tas.Coinbase.Supply.Cmp(big.NewInt(0)) > 0 {
			//set the initial supply to the account
			tas.Balance.Set(&tas.Coinbase.Supply)
		}
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
	app.Type = accountState.Type
	copy(app.IssuerChainId[:], accountState.IssuerChainId[:])
	copy(app.IssuerIdentity[:], accountState.IssuerIdentity[:])
}

const tokenAccountStateLen = 32 + 32 + 32

//GetType is an implemented interface that returns the chain type of the object
func (app *TokenAccountState) GetType() string {
	return string(app.Type)
}

//GetAdiChainPath returns the chain path for the object in the chain.
func (app *TokenAccountState) GetAdiChainPath() string {
	return string(app.AdiChainPath)
}

//GetIssuerIdentity is the identity of the coinbase for the tokentype
func (app *TokenAccountState) GetIssuerIdentity() *types.Bytes32 {
	return &app.IssuerIdentity
}

//GetIssuerChainId is the Issuer's chain id of the coinbase
func (app *TokenAccountState) GetIssuerChainId() *types.Bytes32 {
	return &app.IssuerChainId
}

//SubBalance will subtract a balance form the account.  If this is a coinbase account,
//the balance will not be subtracted.
func (app *TokenAccountState) SubBalance(amt *big.Int) error {
	if amt == nil {
		return fmt.Errorf("invalid input amount specified to subtract from balance")
	}

	if app.IsCoinbaseAccount() {
		if app.Coinbase.Supply.Sign() < 0 {
			//if we have unlimited supply, never subtract balance
			return nil
		}
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
	if amt == nil {
		return fmt.Errorf("invalid input amount specified to add to balance")
	}

	if amt.Sign() <= 0 {
		return fmt.Errorf("amount to add to balance must be a positive amount")
	}

	//do coinbase stuff if this a coinbase account
	if app.Coinbase != nil {
		//check to see if we are a coinbase, and if our circulation mode is burn only,
		//if so, no funds should be added to this account so return with no error
		if app.Coinbase.Mode == types.TokenCirculationMode_Burn {
			return nil
		}

		//check to see if we have an unlimited supply, if so, no tokens are added.
		if app.Coinbase.Supply.Sign() < 0 {
			return nil
		}

		//do a test add to the balance to make sure it doesn't exceed supply.
		if big.NewInt(0).Add(app.GetBalance(), amt).Cmp(&app.Coinbase.Supply) > 0 {
			return fmt.Errorf("attempting to add tokens to coinbase greater than supply")
		}
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
	data := make([]byte, tokenAccountStateLen+len(coinbase))

	i := copy(data[:], app.IssuerIdentity[:])
	i += copy(data[i:], app.IssuerChainId[:])

	app.Balance.FillBytes(data[i:])
	i += 32

	if app.Coinbase != nil {
		copy(data[i:], coinbase)
	}

	return data, nil
}

//UnmarshalBinary will deserialize a byte array
func (app *TokenAccountState) UnmarshalBinary(data []byte) error {
	if len(data) < tokenAccountStateLen {
		return fmt.Errorf("invalid Token Data for unmarshalling %X on chain %X", app.IssuerIdentity, app.IssuerChainId)
	}

	i := copy(app.IssuerIdentity[:], data[:])
	i += copy(app.IssuerChainId[:], data[i:])

	app.Balance.SetBytes(data[i:])
	i += 32

	if len(data) > i {
		coinbase := types.TokenIssuance{}
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

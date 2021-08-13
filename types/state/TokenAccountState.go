package state

import (
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/types"
	"math/big"
)

type tokenAccountState struct {
	Type           string               `json:"type"`
	ChainPath      string               `json:"chain-path"`
	IssuerIdentity types.Bytes32        `json:"issuer-identity"` //need to know who issued tokens, this can be condensed maybe back to adi chain path
	IssuerChainId  types.Bytes32        `json:"issuer-chain-id"` //identity/issue chains both hold the metrics for the TokenRules ... hmm.. do those need to be passed along since those need to be used
	Balance        big.Int              `json:"balance"`
	Coinbase       *types.TokenIssuance `json:"coinbase,omitempty"`
}

type TokenAccountState struct {
	StateEntry
	tokenAccountState
}

func NewTokenAccountState(issuerid []byte, issuerchain []byte, coinbase *types.TokenIssuance) *TokenAccountState {
	tas := TokenAccountState{}
	tas.Type = "AIM-0"
	copy(tas.IssuerIdentity[:], issuerid)
	copy(tas.IssuerChainId[:], issuerchain)
	tas.Coinbase = coinbase
	if tas.IsCoinbaseAccount() {
		if tas.Coinbase.Supply.Cmp(big.NewInt(0)) > 0 {
			//set the initial supply to the account
			tas.Balance.Set(&tas.Coinbase.Supply)
		}
	}
	return &tas
}

const tokenAccountStateLen = 32 + 32 + 32

func (ts *TokenAccountState) GetType() string {
	return ts.Type
}

func (ts *TokenAccountState) GetIssuerIdentity() *types.Bytes32 {
	return &ts.IssuerIdentity
}

func (ts *TokenAccountState) GetIssuerChainId() *types.Bytes32 {
	return &ts.IssuerChainId
}

func (ts *TokenAccountState) SubBalance(amt *big.Int) error {
	if amt == nil {
		return fmt.Errorf("Invalid input amount specified to subtract from balance")
	}

	if ts.IsCoinbaseAccount() {
		if ts.Coinbase.Supply.Sign() < 0 {
			//if we have unlimited supply, never subtract balance
			return nil
		}
	}

	if ts.Balance.Cmp(amt) < 0 {
		return fmt.Errorf("{ \"insufficient-balance\" : { \"available\" : \"%d\" , \"requested\", \"%d\" } }", ts.GetBalance(), amt)
	}

	ts.Balance.Sub(&ts.Balance, amt)
	return nil
}

func (ts *TokenAccountState) GetBalance() *big.Int {
	return &ts.Balance
}

func (ts *TokenAccountState) AddBalance(amt *big.Int) error {
	if amt == nil {
		return fmt.Errorf("Invalid input amount specified to add to balance")
	}

	//do coinbase stuff if this a coinbase account
	if ts.Coinbase != nil {
		//check to see if we are a coinbase, and if our circulation mode is burn only,
		//if so, no funds should be added to this account so return with no error
		if ts.Coinbase.Mode == types.TokenCirculationMode_Burn {
			return nil
		}

		//check to see if we have an unlimited supply, if so, no tokens are added.
		if ts.Coinbase.Supply.Sign() < 0 {
			return nil
		}

		//do a test add to the balance to make sure it doesn't exceed supply.
		if big.NewInt(0).Add(ts.GetBalance(), amt).Cmp(&ts.Coinbase.Supply) > 0 {
			return fmt.Errorf("Attempting to add tokens to coinbase greater than supply")
		}
	}

	ts.Balance.Add(&ts.Balance, amt)
	return nil
}

func (ts *TokenAccountState) IsCoinbaseAccount() bool {
	return ts.Coinbase != nil
}

func (ts *TokenAccountState) MarshalBinary() (ret []byte, err error) {
	var coinbase []byte
	if ts.Coinbase != nil {
		coinbase, err = ts.Coinbase.MarshalBinary()
		if err != nil {
			return nil, err
		}
	}
	data := make([]byte, tokenAccountStateLen+len(coinbase))

	i := copy(data[:], ts.IssuerIdentity[:])
	i += copy(data[i:], ts.IssuerChainId[:])

	ts.Balance.FillBytes(data[i:])
	i += 32

	if ts.Coinbase != nil {
		copy(data[i:], coinbase)
	}

	return data, nil
}

func (ts *TokenAccountState) UnmarshalBinary(data []byte) error {
	if len(data) < tokenAccountStateLen {
		return fmt.Errorf("Invalid Token Data for unmarshalling %X on chain %X", ts.IssuerIdentity, ts.IssuerChainId)
	}

	i := copy(ts.IssuerIdentity[:], data[:])
	i += copy(ts.IssuerChainId[:], data[i:])

	ts.Balance.SetBytes(data[i:])
	i += 32

	if len(data) > i {
		coinbase := types.TokenIssuance{}
		err := coinbase.UnmarshalBinary(data[i:])
		if err != nil {
			return err
		}
		ts.Coinbase = &coinbase
	}

	return nil
}

func (app *TokenAccountState) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, app.tokenAccountState)
}

func (app *TokenAccountState) MarshalJSON() ([]byte, error) {
	return json.Marshal(app.tokenAccountState)
}

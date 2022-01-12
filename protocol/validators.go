package protocol

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/tendermint/tendermint/crypto/ed25519"
)


func NewValidators(operator string, pubKey ed25519.PubKey, description ValidatorDescription) (ValidatorType, error) {
	return ValidatorType{
		OperatorAddress: operator,
		ConsensusPubKey: pubKey,
		Jailed: false,
		Status: 1,
		Tokens: 0,
		DelegatorShares: 0,
		Description: description,
		UnStakingHeight: 0,
		UnStakingTime: time.Unix(0, 0),
		Commission: NewCommission(0, 0, 0),

	}, nil
}


func NewCommission(rate, maxRate, maxChangeRate uint64) Commission {
	return Commission{
	CommissionRates: NewCommissionRates(rate, maxRate, maxChangeRate),
	}
}


func NewCommissionRates(rate, maxRate, maxChangeRate uint64) CommissionRates {
	return CommissionRates{
		Rate: rate,
		MaxRate: maxRate,
		MaxChangeRate: maxChangeRate,
	}
}


func NewCommissionRatesWithTime(rate, maxRate, maxChangeRate uint64) Commission {
	return Commission{
		CommissionRates: NewCommissionRates(rate, maxRate, maxChangeRate),
		}
}

func (c CommissionRates) Validate() error {
	if c.MaxRate < c.Rate {
		return fmt.Errorf("max rate cannot be less than the rate")
	}
	if c.MaxRate < 0 || c.Rate < 0 || c.MaxChangeRate < 0 {
		return fmt.Errorf("commission parameters must be positive")
	}
	return nil
}


type Vals []ValidatorType

func (v Vals) String() (out string) {
	for _, val := range v {
		out += fmt.Sprintf("%v\n", val)
	}
	return strings.TrimSpace(out)
}

type ValsByVotingPower []ValidatorType

func (vals ValsByVotingPower) Len() int {
	return len(vals)
}
/*
func (vals ValsByVotingPower) Less(i, j int, r big.Int) bool {
	if vals[i].ConsensusPower(r) == vals[j].ConsensusPower(r) {
		addyI, errI := vals[i].GetConsensusAddress()
		addyJ, errJ := vals[j].GetConsensusAddress()
		// if any return error, then return false
		if errI != nil || errJ != nil {
			return false
		}
		return bytes.Compare(addyI, addyJ) == -1
	}
	return vals[i].ConsensusPower(r) > vals[j].ConsensusPower(r)

}

*/
func (v ValidatorType) IsJailed() bool { return v.Jailed }
func (v ValidatorType) GetMoniker() string { return v.Description.Moniker }
func (v ValidatorType) GetStatus() int64 { return v.Status }
func (v ValidatorType) GetOperator() string { 
	if v.OperatorAddress == "" {
		return ""
 }
 	addr := v.OperatorAddress
 	return addr 
}

 func (v ValidatorType) SetInitCommission(commission Commission) (ValidatorType, error) {
	if err := commission.CommissionRates.Validate(); err != nil {
		return v, err
	}
	v.Commission = commission
	return v, nil
}

//swap
func (v ValidatorType) IsBonded() bool {
	return v.GetStatus() == 0
}

func (v ValidatorType) IsUnbonded() bool {
	return v.GetStatus() == 1
}

func (v ValidatorType) IsUnbonding() bool {
	return v.GetStatus() == 2
}

func NewDescription(moniker, identity, website, details string) ValidatorDescription {
	return ValidatorDescription{
		Moniker: moniker,
		Identity: identity,
		Website: website,
		Details: details,
	}
}

// ConsensusPower gets the consensus-engine power. Aa reduction of 10^6 from
// validator tokens is applied
func (v ValidatorType) ConsensusPower(r big.Int) int64 {
	if v.IsBonded() {
		return v.PotentialConsensusPower(r)
	}

	return 0
}

// PotentialConsensusPower returns the potential consensus-engine power.
func (v ValidatorType) PotentialConsensusPower(r big.Int) int64 {
	return TokensToConsensusPower(v.Tokens, r)
}

// TokensToConsensusPower - convert input tokens to potential consensus-engine power
func TokensToConsensusPower(tokens Int, powerReduction Int) int64 {
	return (tokens.Quo(powerReduction)).Int64()
}

func (d ValidatorDescription) UpdateDescription(d1 ValidatorDescription) (ValidatorDescription, error) {
	if d1.Moniker == "" {
		d1.Moniker = d.Moniker
		}
	
	if d1.Identity == "" {
		d1.Identity = d.Identity
		}

	if d1.Website == "" {
		d1.Website = d.Website
		}

	return NewDescription(d1.Moniker, d1.Identity, d1.Website, d1.Details), nil



}

func (v ValidatorType) GetConsensusAddress() (conAdd []byte, err error) {
	pk, err := v.ConsensusPublicKey()
	if err != nil {
		return nil, err
	}
	return pk.Address(), nil

}

func (v ValidatorType) GetConsensusPower(r big.Int) big.Int {
	return v.
}

//ConsensusPublicKey returns validator's public key as ed25519.PubKey
func (v ValidatorType) ConsensusPublicKey() (ed25519.PubKey, error) {
	//return v.ConsensusPubKey
	pk, ok := v.ConsensusPubKey.GetPubKey().(ed25519.PubKey)
	if !ok {
		panic("Validator.ConsensusPublicKey() should return an ed25519.PubKey")
	}
	return pk, nil
}


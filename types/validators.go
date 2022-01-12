package types

import (
	"bytes"
	"fmt"
	"math/big"
	"strings"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/tendermint/tendermint/crypto/ed25519"
	tmcrypto "github.com/tendermint/tendermint/crypto/ed25519"
)

//types
type BondedStatus int32
const (
	Unknown BondedStatus = 0
	Unbonded BondedStatus = 1
	Unbonding BondedStatus = 2
	Bonded BondedStatus = 3
)
var BondedStatus_name = map[int32]string{
	0: "BOND_STATUS_UNKNOWN",
	1: "BOND_STATUS_UNBONDED",
	2: "BOND_STATUS_UNBONDING",
	3: "BOND_STATUS_BONDED",
}
var BondedStatus_value = map[string]int32{
	"BOND_STATUS_UNKNOWN": 0,
	"BOND_STATUS_UNBONDED": 1,
	"BOND_STATUS_UNBONDING": 2,
	"BOND_STATUS_BONDED": 3,
}

const (
	MaxMonikerLength = 100
	MaxIdentityLength = 3000
	MaxWebsiteLength = 255
	MaxDetailsLength = 32768
)

var (
	BondedStatusUnknown = BondedStatus_name[(int32(Unknown))]
	BondedStatusUnbonded = BondedStatus_name[(int32(Unbonded))]
	BondedStatusUnbonding = BondedStatus_name[(int32(Unbonding))]
	BondedStatusBonded = BondedStatus_name[(int32(Bonded))]
)

type Validatorz interface {
    isJailed() bool
    GetMoniker() string
    GetStatus() protocol.BondStatus
    IsBonded() bool
    IsUnbonded() bool
    IsUnbonding() bool
    GetOperator() string    //operator address to recive return ACME
    ConsensusPubKey() (ed25519.PubKey, error)
    TmPublicKey() (tmcrypto.PubKey, error)
    GetConsensusAddress() string
    GetACME() big.Int
    GetBondedACME() big.Int
    GetVotingPower(big.Int) big.Int
    GetCommission() uint64 //needs to be decimal

}
var _ Validatorz = protocol.ValidatorType{}


func NewValidator(operator string, pubKey ed25519.PubKey, acme big.Int, bondedStatus BondedStatus, commission uint64, details string, website string, identity string, moniker string) (protocol.ValidatorType, error) {
	return protocol.ValidatorType{
		Operator: operator,
		PubKey: pubKey,
		Acme: acme,
		BondedStatus: bondedStatus,
		Commission: commission,
		Details: details,
		Website: website,
		Identity: identity,
		Moniker: moniker,
	}, nil
}



type Vals []protocol.ValidatorType

func (v Vals) String() (out string) {
	for _, val := range v {
		out += fmt.Sprintf("%v\n", val)
	}
	return strings.TrimSpace(out)
}

type ValsByVotingPower []protocol.ValidatorType

func (vals ValsByVotingPower) Len() int {
	return len(vals)
}

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


func (v protocol.ValidatorType) IsJailed() bool { return v.Jailed }
func (v protocol.ValidatorType) GetMoniker() bool { return v.Description.Moniker }
func (v protocol.ValidatorType) GetStatus() BondedStatus { return v.Status }
//func (v protocol.ValidatorType) GetOperator() string { 
//	if v.Operator == "" {
//		return nil
 //}
 //addr, err 


//swap
func (v protocol.ValidatorType) IsBonded() bool {
	return v.GetStatus() == Bonded
}

func (v protocol.ValidatorType) IsUnbonded() bool {
	return v.GetStatus() == Unbonded
}

func (v protocol.ValidatorType) IsUnbonding() bool {
	return v.GetStatus() == Unbonding
}

func NewDescription(moniker, identity, website, details string) protocol.ValidatorDescription {
	return protocol.ValidatorDescription{
		Moniker: moniker,
		Identity: identity,
		Website: website,
		Details: details,
	}
}

func (d protocol.ValidatorDescription) UpdateDescription(d1 protocol.ValidatorDescription) (protocol.ValidatorDescription, error) {
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

func (v protocol.ValidatorType) ConsensusPower(r big.Int) big.Int {
	return v.Acme.Mul(v.Acme, r)
}

//ConsensusPublicKey returns validator's public key as ed25519.PubKey
func (v protocol.ValidatorType) ConsensusPublicKey() (ed25519.PubKey, error) {
	//return v.ConsensusPubKey
	pk, ok := v.ConsensusPubKey.GetPubKey().(ed25519.PubKey)
	if !ok {
		panic("Validator.ConsensusPublicKey() should return an ed25519.PubKey")
	}
	return pk, nil
}


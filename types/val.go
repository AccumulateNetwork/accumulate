package types

import (
	"math/big"

	"github.com/AccumulateNetwork/accumulate/protocol"
	tmcrypto "github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/crypto/ed25519"
)

//types
type BondStatus int64
var (
	Unknown BondStatus = 0
	Unbonded BondStatus = 1
	Unbonding BondStatus = 2
	Bonded BondStatus = 3
)
var BondedStatus_name = map[int64]string{
	0: "BOND_STATUS_UNKNOWN",
	1: "BOND_STATUS_UNBONDED",
	2: "BOND_STATUS_UNBONDING",
	3: "BOND_STATUS_BONDED",
}
var BondedStatus_value = map[string]int64{
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
	BondedStatusUnknown = BondedStatus_name[(int64(Unknown))]
	BondedStatusUnbonded = BondedStatus_name[(int64(Unbonded))]
	BondedStatusUnbonding = BondedStatus_name[(int64(Unbonding))]
	BondedStatusBonded = BondedStatus_name[(int64(Bonded))]
)


type Delegate interface {
	GetDelegatorAddress() []byte
	GetValidatorAddress() []byte
	GetShares() *big.Int
}

type Validatorz interface {
    isJailed() bool 									// Validator can be jailed for acting up
    GetMoniker() string									// Validator's name	
    GetStatus() BondStatus								// Validator's status [bonded/unbonded/unbonding]
    IsBonded() bool
    IsUnbonded() bool
    IsUnbonding() bool
    GetOperator() string    							//operator address to receive return ACME
    ConsensusPubKey() (ed25519.PubKey, error)			// Validator's public key
    TmPublicKey() (tmcrypto.PubKey, error)
    GetConsensusAddress() string						// Validator's consensus address
    GetACME() big.Int									// Validator's ACME
    GetBondedACME() big.Int								// Validator's staked ACME
    GetVotingPower(big.Int) big.Int						// Amount of Voting Power the operator has
    GetCommission() uint64 								//needs to be decimal
	GetDelegatorShares() big.Int						// Amount of shares the operator has

}


func GetValidator(addr string) (validator protocol.ValidatorType, found bool) {
	return
}


func SetValidator(addr string, validator protocol.ValidatorType) {
	
}




type ValidatorSet interface {
	Validator(valaddr string) Validatorz   			// Get a validator by address
	ValidatorByPubKey(pubkey []byte) Validatorz 	// Get a validator by pubkey
	TotalStakedACME() *big.Int						// Total amount of ACME staked in the validator set
	StakedSupply() *big.Int							// Total staking supply

	Jail(valaddr string) 							// Jail a validator
	Release(valaddr string) 						// Release a jailed validator
	Slash(valaddr string, amount int64) big.Int		// Slash a validator

	MaximumValidators() uint32 						// Maximum number of validators

	IterateValidators(func(index int64, val Validatorz) (stop bool))
	IterateStakedValidatorsByVotingPower(func(index int64, val Validatorz) (stop bool))

}


func IterateValidators(fn func(index int64, val Validatorz) (stop bool)) {
}

func IterateStakedValidatorsByVotingPower(fn func(index int64, val Validatorz) (stop bool)) {
}


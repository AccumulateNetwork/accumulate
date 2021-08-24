package accnode

import (
	"github.com/AccumulateNetwork/accumulated/blockchain/tendermint"
	"github.com/AccumulateNetwork/accumulated/blockchain/validator"
)

func CreateAccumulateBVC(config string, path string) (*tendermint.AccumulatorVMApplication, error) {
	//create a AccumulatorVM
	acc := tendermint.NewAccumulatorVMApplication(config, path)

	//initiate a transaction for token transfer
	atktx := validator.NewTokenTransactionValidator()
	err := acc.AddValidator(&atktx.ValidatorContext)
	if err != nil {
		return nil, err
	}

	//support validation for a deposit into an account.
	synthval := validator.NewSyntheticTransactionDepositValidator()
	err = acc.AddValidator(&synthval.ValidatorContext)
	if err != nil {
		return nil, err
	}

	//support validation to issue a new token
	tokissuance := validator.NewTokenIssuanceValidator()
	err = acc.AddValidator(&tokissuance.ValidatorContext)
	if err != nil {
		return nil, err
	}

	//support validation for creating a token url
	tokchain := validator.NewTokenChainCreateValidator()
	err = acc.AddValidator(&tokchain.ValidatorContext)
	if err != nil {
		return nil, err
	}

	//create identity validator
	idval := validator.NewAdiChain()
	err = acc.AddValidator(&idval.ValidatorContext)
	if err != nil {
		return nil, err
	}

	//add validator for validating the synthetic identity create tx
	synthidentity := validator.NewSyntheticIdentityStateCreateValidator()
	err = acc.AddValidator(&synthidentity.ValidatorContext)
	if err != nil {
		return nil, err
	}

	//entryval := validator.NewEntryValidator()
	//acc.AddValidator(&entryval.ValidatorContext)

	//this is only temporary to handle leader sending messages to the DBVC to produce receipts.
	//there will only be one of these in the network
	dbvcval := validator.NewBVCLeader()
	err = acc.AddValidator(&dbvcval.ValidatorContext)
	if err != nil {
		return nil, err
	}

	go acc.Start()

	acc.Wait()
	return acc, nil
}

func CreateAccumulateDBVC(config string, path string) *tendermint.AccumulatorVMApplication {
	//create a AccumulatorVM
	acc := tendermint.NewAccumulatorVMApplication(config, path)

	dbvcval := validator.NewBVCLeader()
	acc.AddValidator(&dbvcval.ValidatorContext)

	go acc.Start()

	acc.Wait()
	return acc
}

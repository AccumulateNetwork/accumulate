package accnode

import (
	"github.com/AccumulateNetwork/accumulated/blockchain/tendermint"
	"github.com/AccumulateNetwork/accumulated/blockchain/validator"
)

func CreateAccumulateBVC(config string, path string) *tendermint.AccumulatorVMApplication {
	//create a AccumulatorVM
	acc := tendermint.NewAccumulatorVMApplication(config, path)

	//initiate a transaction for token transfer
	atktx := validator.NewTokenTransactionValidator()
	acc.AddValidator(&atktx.ValidatorContext)

	//support validation for a deposit into an account.
	synthval := validator.NewSyntheticTransactionDepositValidator()
	acc.AddValidator(&synthval.ValidatorContext)

	//support validation to issue a new token
	tokissuance := validator.NewTokenIssuanceValidator()
	acc.AddValidator(&tokissuance.ValidatorContext)

	//support validation for creating a token url
	tokchain := validator.NewTokenChainCreateValidator()
	acc.AddValidator(&tokchain.ValidatorContext)

	//create identity validator
	idval := validator.NewCreateIdentityValidator()
	acc.AddValidator(&idval.ValidatorContext)

	//add validator for validating the synthetic identity create tx
	synthidentity := validator.NewCreateIdentityValidator()
	acc.AddValidator(&synthidentity.ValidatorContext)

	//entryval := validator.NewEntryValidator()
	//acc.AddValidator(&entryval.ValidatorContext)

	//this is only temporary to handle leader sending messages to the DBVC to produce receipts.
	//there will only be one of these in the network
	dbvcval := validator.NewBVCLeader()
	acc.AddValidator(&dbvcval.ValidatorContext)

	go acc.Start()

	acc.Wait()
	return acc
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

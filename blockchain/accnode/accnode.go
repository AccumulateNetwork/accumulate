package accnode

import (
	"github.com/AccumulateNetwork/accumulated/blockchain/tendermint"
	"github.com/AccumulateNetwork/accumulated/blockchain/validator"
)

func CreateAccumulateBVC(config string, path string) *tendermint.AccumulatorVMApplication {
	//create a AccumulatorVM
	acc := tendermint.NewAccumulatorVMApplication(config, path)

	atktx := validator.NewTokenTransactionValidator()
	acc.AddValidator(&atktx.ValidatorContext)

	//create and add some validators for known types
	//fct := validator.NewFactoidValidator()
	//acc.AddValidator(&fct.ValidatorContext)

	synthval := validator.NewSyntheticTransactionValidator()
	acc.AddValidator(&synthval.ValidatorContext)

	idval := validator.NewCreateIdentityValidator()
	acc.AddValidator(&idval.ValidatorContext)

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

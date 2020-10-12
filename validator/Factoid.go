package validator


import (
	"fmt"
	cfg "github.com/tendermint/tendermint/config"
	nm "github.com/tendermint/tendermint/node"
	dbm "github.com/tendermint/tm-db"

	//"github.com/FactomProject/factomd/common/factoid"
	factom "github.com/Factom-Asset-Tokens/factom"
)


type FactoidValidator struct{
	ValidatorInterface
	KvStoreDB dbm.DB
	AccountsDB dbm.DB
}



func (v *FactoidValidator) InitDBs(config *cfg.Config, dbProvider nm.DBProvider) (err error) {

	v.AccountsDB, err = dbProvider(&nm.DBContext{"fctaccounts", config})
	if err != nil {
		return
	}

	v.KvStoreDB, err = dbProvider(&nm.DBContext{"fctkvStore", config})
	if err != nil {
		return
	}

	return
}

func (v *FactoidValidator) Validate(tx []byte) uint32 {
	//if pass then send to accumulator.
	//make sure the factoid transaction format is valid.

    //t := new(factoid.Transaction)
	t := new(factom.Transaction)
	err := t.UnmarshalBinary(tx)

	if err != nil {
		fmt.Printf("Invalid Transaction")
	}

	//make sure there is enough available balance
//    inp := t.GetInputs()
    //inp returns a transaction interface.

	//if so, record transaction in factoid database, and pass to validator
	return 0
}

func NewFactoidValidator() *FactoidValidator {
	v := FactoidValidator{}
	return &v
}

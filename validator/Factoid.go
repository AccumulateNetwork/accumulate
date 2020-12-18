package validator


import (
	"fmt"
	cfg "github.com/tendermint/tendermint/config"
	//nm "github.com/tendermint/tendermint/node"
	nm "github.com/AccumulateNetwork/accumulated/vbc/node"
	dbm "github.com/tendermint/tm-db"
	"time"

	//"time"

	//"github.com/magiconair/properties/assert"

	//"github.com/FactomProject/factomd/common/factoid"
	factom "github.com/Factom-Asset-Tokens/factom"
)


type FactoidValidator struct{
	ValidatorContext
	KvStoreDB dbm.DB
	AccountsDB dbm.DB
}

func NewFactoidValidator() *FactoidValidator {
	v := FactoidValidator{}
	//need the chainid, then hash to get first 8 bytes to make the chainid.
	//by definition a chainid of a factoid block is
	//000000000000000000000000000000000000000000000000000000000000000f
	//the id will be 0x0000000f
	chainid := "000000000000000000000000000000000000000000000000000000000000000f"
	v.SetInfo(chainid,"factoid")
	v.ValidatorContext.ValidatorInterface = &v
	return &v
}

func (v *FactoidValidator) InitDBs(config *cfg.Config, dbProvider nm.DBProvider) (err error) {

	//v.AccountsDB.Get()
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

func (v *FactoidValidator) Validate(data []byte) error {
	//if pass then send to accumulator.
	//var fblock := factom.FBlock{}
	//create a new block

	tx := factom.Transaction{}

	err := tx.UnmarshalBinary(data)
	if err != nil {
		fmt.Printf("Invalid FCT Transaction")
		return err
	}



	timeofvalidity := time.Duration(2) * time.Minute//transaction good for only 2 minutes
	elapsed := tx.TimestampSalt.Sub(*v.GetCurrentTime()) * time.Minute
	if elapsed > timeofvalidity || elapsed < 0 {
		//need to log instaed
		fmt.Printf("Invalid FCT Transaction: Timestamp out of bounds")
		return nil
	}

	//need to check balances
	//inp := tx.FCTInputs


	//
/*
	fblock := factom.FBlock{}

	err := fblock.UnmarshalBinary(tx)

	if err != nil {
		fmt.Printf("Invalid FCT Transaction")
		return 0
	}

	//require.NoError(err)
	if  fblock.BodyMR != nil {
		fmt.Printf("Invalid BodyMR")
		return 0
	}
*/
	//if fblock.Timestamp

	//require.NotNil(f.BodyMR)

	//data, err := f.MarshalBinary()
	//require.NoError(err)
	//assert.Equal(test.Data, data)

	//assert.Equal(test.BodyMR[:], f.BodyMR[:])

	//assert.Equal(test.KeyMr[:], f.KeyMR[:])

	//if len(test.Expansion) > 0 {
	//	assert.Equal(test.Expansion[:],
	//		f.Expansion[:])

	//check the timestamp to make sure it is in the valid range.

	//make sure there is enough available balance
	//    inp := t.GetInputs()

    //inp returns a transaction interface.

	//if so, record transaction in factoid database, and pass to validator
	return nil
}


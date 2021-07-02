package validator


import (
	"fmt"
	pb "github.com/AccumulateNetwork/accumulated/proto"
	//cfg "github.com/tendermint/tendermint/config"
	//nm "github.com/tendermint/tendermint/node"
	//nm "github.com/AccumulateNetwork/accumulated/vbc/node"
	dbm "github.com/tendermint/tm-db"
	"time"

	//"time"

	//"github.com/magiconair/properties/assert"

	//"github.com/FactomProject/factomd/common/factoid"
	"github.com/AccumulateNetwork/accumulated/factom"
	"math/rand"
)


type FactoidValidator struct{
	ValidatorContext
	KvStoreDB dbm.DB
	AccountsDB dbm.DB
	timeofvalidity time.Duration
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
	v.timeofvalidity = time.Duration(2) * time.Minute //transaction good for only 2 minutes
	return &v
}
//
//func (v *FactoidValidator) InitDBs(config *cfg.Config, dbProvider nm.DBProvider) (err error) {
//
//	//v.AccountsDB.Get()
//	v.AccountsDB, err = dbProvider(&nm.DBContext{"fctaccounts", config})
//	if err != nil {
//		return
//	}
//
//	v.KvStoreDB, err = dbProvider(&nm.DBContext{"fctkvStore", config})
//	if err != nil {
//		return
//	}
//
//	return
//}

func (v *FactoidValidator) Check(ins uint32, p1 uint64, p2 uint64, data []byte) error {
	tx := factom.Transaction{}
	err := tx.UnmarshalBinary(data)
	if err != nil {
		return err
	}



	elapsed := tx.TimestampSalt.Sub(*v.GetCurrentTime()) * time.Minute
	if elapsed > v.timeofvalidity || elapsed < 0 {
		//need to log instaed
		return fmt.Errorf("Invalid FCT Transaction: Timestamp out of bounds")
	}

    return nil
}
func (v *FactoidValidator) processFctTx(data []byte) ([]byte, error) {

	tx := factom.Transaction{}
	err := tx.UnmarshalBinary(data)
	if err != nil {
		fmt.Printf("Invalid FCT Transaction")
		return nil,err
	}



	elapsed := tx.TimestampSalt.Sub(*v.GetCurrentTime()) * time.Minute
	if elapsed > v.timeofvalidity || elapsed < 0 {
		//need to log instaed
		err := fmt.Errorf("Invalid FCT Transaction: Timestamp out of bounds")
		return nil, err
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

	return nil, err
}

func (v *FactoidValidator) processEcTx(data []byte) ([]byte, error) {
	tx := factom.Transaction{}
	err := tx.UnmarshalBinary(data)

	return nil, err
}


func (v *FactoidValidator) Validate(ins uint32, p1 uint64, p2 uint64, data []byte) (*ResponseValidateTX,error) {
	//if pass then send to accumulator.
	//var fblock := factom.FBlock{}
	//create a new block

	tx := factom.Transaction{}
	err := tx.UnmarshalBinary(data)
	if err != nil {
		fmt.Printf("Invalid FCT Transaction")
		return nil,err
	}


	elapsed := tx.TimestampSalt.Sub(*v.GetCurrentTime()) * time.Minute
	if elapsed > v.timeofvalidity || elapsed < 0 {
		//need to log instaed
		err := fmt.Errorf("Invalid FCT Transaction: Timestamp out of bounds")
		return nil, err
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

	///build an AccumulateTransaction,
	// atktx := AccumulateTransaction.MarshalBinary()
	// atkchainaddr := AccumulateTransaction.ChainAddr()
	// ins := AccumulateTransaction.Instruction()

	ret := ResponseValidateTX{}
	submissions := make([]pb.Submission,2)

    atktx := make([]byte, 64)
    atkchainaddr := uint64(0x0000000000000000)
    //for now just read random stuff
	rand.Read(atktx)
	atkins := uint32(0) //
	submissions[1] = pb.Submission{
		Address: atkchainaddr,
		Type:    pb.Submission_Token_Transaction,
		Instruction: atkins,
		Param1: 0,
		Param2: 0,
		Data:    atktx,//data does not contain an RCD will be added
	}

	return &ret,err
}


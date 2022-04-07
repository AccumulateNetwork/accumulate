package pmt

import (
	"crypto/sha256"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/badger"
)

func TestSaveState(t *testing.T) {
	numberEntries := 50732 //               A pretty reasonable sized BPT

	DirName, err := ioutil.TempDir("", "AccDB")
	require.Nil(t, err, "failed to create directory")
	defer os.RemoveAll(DirName)

	BDB, err := badger.New(DirName+"/add", nil)
	defer BDB.Close()
	require.Nil(t, err, "failed to create db")

	storeTx := BDB.Begin(true)           // and begin its use.
	bptManager := NewBPTManager(storeTx) // Create a BptManager.  We will create a new one each cycle.
	bpt := bptManager.Bpt                //     Build a BPT
	var keys, values common.RandHash     //     use the default sequence for keys
	values.SetSeed([]byte{1, 2, 3})      //     use a different sequence for values
	for i := 0; i < numberEntries; i++ { // For the number of Entries specified for the BPT
		chainID := keys.NextAList() //      Get a key, keep a list
		value := values.GetRandBuff(int(values.GetRandInt64() % 2048))
		hash := sha256.Sum256(value)
		storeTx.Put(hash, value)
		bpt.Insert(chainID, hash) //      Insert the Key with the value into the BPT
	}
	bptManager.DBManager.Commit()
	bpt.manager.DBManager = BDB.Begin(true)

	err = bpt.SaveSnapshot(DirName + "/SnapShot")

	Bpt := NewBPTManager()
	require.Nil(t, err, "snapshot failed")
}

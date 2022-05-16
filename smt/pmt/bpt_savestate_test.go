package pmt

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/badger"
)

func TestSaveState(t *testing.T) {

	numberEntries := 5001 //               A pretty reasonable sized BPT

	DirName, err := ioutil.TempDir("", "AccDB")
	require.Nil(t, err, "failed to create directory")
	defer os.RemoveAll(DirName)

	BDB, err := badger.New(DirName+"/add", nil)
	require.Nil(t, err, "failed to create db")
	defer BDB.Close()

	storeTx := BDB.Begin(true)           // and begin its use.
	bptManager := NewBPTManager(storeTx) // Create a BptManager.  We will create a new one each cycle.
	bpt := bptManager.Bpt                //     Build a BPT
	var keys, values common.RandHash     //     use the default sequence for keys
	values.SetSeed([]byte{1, 2, 3})      //     use a different sequence for values
	for i := 0; i < numberEntries; i++ { // For the number of Entries specified for the BPT
		chainID := keys.NextAList() //      Get a key, keep a list
		value := values.GetRandBuff(int(values.GetRandInt64() % 100))
		hash := sha256.Sum256(value)
		err := storeTx.Put(hash, value)
		require.NoError(t, err, "fail")
		bpt.Insert(chainID, hash) //      Insert the Key with the value into the BPT
	}
	err = bptManager.Bpt.Update()
	require.NoError(t, err, "fail")
	err = bptManager.DBManager.Commit()
	require.NoError(t, err, "fail")
	storeTx = BDB.Begin(true)
	bpt.manager.DBManager = storeTx

	f, err := os.Create(filepath.Join(DirName, "SnapShot"))
	require.NoError(t, err)
	defer f.Close()

	err = bpt.SaveSnapshot(f, func(key storage.Key, hash [32]byte) ([]byte, error) {
		return storeTx.Get(hash)
	})
	require.NoErrorf(t, err, "%v", err)

	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)

	bptMan := NewBPTManager(nil)
	err = bptMan.Bpt.LoadSnapshot(f, func(key storage.Key, hash [32]byte, reader ioutil2.SectionReader) error {
		value, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		valueHash := sha256.Sum256(value)
		if hash != valueHash {
			return fmt.Errorf("hash does not match for key %X", key)
		}
		return storeTx.Put(key, hash[:])
	})
	require.NoErrorf(t, err, "%v", err)
	err = bptMan.Bpt.Update()
	require.True(t, bpt.Root.Hash == bptMan.Bpt.RootHash, "fail")
	require.Nil(t, err, "snapshot failed")
}

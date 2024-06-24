// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func TestSaveState(t *testing.T) {

	numberEntries := 5001 //               A pretty reasonable sized BPT

	DirName, err := os.MkdirTemp("", "AccDB")
	require.Nil(t, err, "failed to create directory")
	defer os.RemoveAll(DirName)

	BDB, err := badger.New(DirName + "/add")
	require.Nil(t, err, "failed to create db")
	defer BDB.Close()

	storeTx := BDB.Begin(nil, true)               // and begin its use.
	store := keyvalue.RecordStore{Store: storeTx} //
	bpt := newBPT(nil, nil, store, nil, "BPT")    // Create a BptManager.  We will create a new one each cycle.
	var keys, values common.RandHash              //     use the default sequence for keys
	values.SetSeed([]byte{1, 2, 3})               //     use a different sequence for values
	for i := 0; i < numberEntries; i++ {          // For the number of Entries specified for the BPT
		chainID := keys.NextAList() //      Get a key, keep a list
		value := values.GetRandBuff(int(values.GetRandInt64() % 100))
		hash := sha256.Sum256(value)
		err := storeTx.Put(record.KeyFromHash(hash), value)
		require.NoError(t, err)
		err = bpt.Insert(record.KeyFromHash(chainID), hash[:]) //      Insert the Key with the value into the BPT
		require.NoError(t, err)
	}
	require.NoError(t, bpt.Commit())
	storeTx = BDB.Begin(nil, true)
	store = keyvalue.RecordStore{Store: storeTx}
	bpt = newBPT(nil, nil, store, nil, "BPT")

	f, err := os.Create(filepath.Join(DirName, "SnapShot"))
	require.NoError(t, err)
	defer f.Close()

	err = SaveSnapshotV1(bpt, f, func(key storage.Key, hash [32]byte) ([]byte, error) {
		return storeTx.Get(record.KeyFromHash(hash))
	})
	require.NoError(t, err)

	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)

	kvs2 := memory.New(nil).Begin(nil, true)
	store2 := keyvalue.RecordStore{Store: kvs2}
	bpt2 := newBPT(nil, nil, store2, nil, "BPT")
	err = LoadSnapshotV1(bpt2, f, func(key storage.Key, hash [32]byte, reader ioutil2.SectionReader) error {
		value, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		valueHash := sha256.Sum256(value)
		if hash != valueHash {
			return fmt.Errorf("hash does not match for key %X", key)
		}
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, bpt.Commit())
	r1, err := bpt.GetRootHash()
	require.NoError(t, err)
	r2, err := bpt2.GetRootHash()
	require.NoError(t, err)
	require.Equal(t, r1, r2)
}

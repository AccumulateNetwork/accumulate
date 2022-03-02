package database

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"golang.org/x/exp/rand"
)

func TestDBManager_TransactionsBadger(t *testing.T) {

	dbManager := new(Manager)

	dir, err := ioutil.TempDir("", "badger")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	if err := dbManager.Init("badger", dir, nil); err != nil {
		t.Error(err)
	} else {
		writeAndRead(t, dbManager)
		writeAndReadBatch(t, dbManager)

	}
}

func TestDBManager_TransactionsMemory(t *testing.T) {

	dbManager := new(Manager)
	_ = dbManager.Init("memory", "", nil)
	writeAndReadBatch(t, dbManager)
	writeAndRead(t, dbManager)
	dbManager.Close()
}

func randSHA() [32]byte {
	v := rand.Uint64()
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return sha256.Sum256(b[:])
}

func writeAndReadBatch(t *testing.T, dbManager *Manager) {
	const cnt = 10 // how many test values used

	// Buckets are "a" "b" and "c"

	type ent struct { // Keep a history
		Key   storage.Key
		Value []byte
	}
	var submissions []ent // Every key/value submitted goes here, in order
	add := func() {       // Generate a key/value pair, add to db, and record
		key := randSHA()   // Generate next key
		value := randSHA() // Generate next value

		submissions = append(submissions, ent{Key: storage.MakeKey("a", key), Value: value[:]}) // Keep a history
		dbManager.PutBatch(storage.MakeKey("a", key[:]), value[:])                              // Put into the database
	}

	// Now this is the actual test
	for i := byte(0); i < cnt; i++ {
		add()
		for i, pair := range submissions {
			require.NotNil(t, dbManager.txCache[pair.Key], "Entry %d missing", i)
			require.Equal(t, pair.Value[:], dbManager.txCache[pair.Key], "Entry %d has wrong value", i)
		}
	}

	dbManager.EndBatch()
	for i, pair := range submissions {
		DBValue, e := dbManager.DB.Get(pair.Key)
		require.Nil(t, e, "Get Failed")
		require.NotNil(t, DBValue, "Entry %d missing", i)
		require.Equal(t, pair.Value[:], DBValue, "Entry %d has wrong value", i)
	}

}

func writeAndRead(t *testing.T, dbManager *Manager) {
	d1 := []byte{1, 2, 3}
	d2 := []byte{2, 3, 4}
	d3 := []byte{3, 4, 5}
	_ = dbManager.Put(storage.MakeKey("a", "", "horse"), d1)
	_ = dbManager.Put(storage.MakeKey("b", "", "horse"), d2)
	_ = dbManager.Put(storage.MakeKey("c", "", "horse"), d3)
	v1, e1 := dbManager.Get(storage.MakeKey("a", "", "horse"))
	require.Nil(t, e1, "could not retrieve value")
	v2, e2 := dbManager.Get(storage.MakeKey("b", "", "horse"))
	require.Nil(t, e2, "could not retrieve value")
	v3, e3 := dbManager.Get(storage.MakeKey("c", "", "horse"))
	require.Nil(t, e3, "could not retrieve value")

	if !bytes.Equal(d1, v1) || !bytes.Equal(d2, v2) || !bytes.Equal(d3, v3) {
		t.Error("All values should be equal")
	}

	for i := 0; i < 10; i++ {
		dbManager.PutBatch(storage.MakeKey("a", "", common.Int64Bytes(int64(i))), []byte(fmt.Sprint(i)))
	}

	dbManager.EndBatch()

	// Sort that I can read all thousand entries
	for i := 0; i < 10; i++ {
		value, e := dbManager.Get(storage.MakeKey("a", "", common.Int64Bytes(int64(i))))
		require.Nil(t, e, "could not retrieve value")
		eValue := []byte(fmt.Sprint(i))

		if !bytes.Equal(value, eValue) {
			t.Error("failed to retrieve value ", eValue, " at: ", i, " got ", value)
		}
	}

}

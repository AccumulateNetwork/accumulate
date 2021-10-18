package database_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/smt/storage/database"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/rand"
)

func TestDBManager_TransactionsBadger(t *testing.T) {

	dbManager := new(database.Manager)

	dir, err := ioutil.TempDir("", "badger")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	if err := dbManager.Init("badger", dir); err != nil {
		t.Error(err)
	} else {
		writeAndRead(t, dbManager)
		writeAndReadBatch(t, dbManager)

	}
}

func TestDBManager_TransactionsMemory(t *testing.T) {

	dbManager := new(database.Manager)
	_ = dbManager.Init("memory", "")
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

func writeAndReadBatch(t *testing.T, dbManager *database.Manager) {
	const cnt = 10 // how many test values used

	dbManager.AddBucket("a") // add a bucket for putting data into the db
	type ent struct {        // Keep a history
		Key   [32]byte
		Value []byte
	}
	var submissions []ent // Every key/value submitted goes here, in order
	add := func() {       // Generate a key/value pair, add to db, and record
		key := randSHA()   // Generate next key
		value := randSHA() // Generate next value

		theKey := dbManager.GetKey("a", "", key[:])
		submissions = append(submissions, ent{Key: theKey, Value: value[:]}) // Keep a history

		dbManager.PutBatch("a", "", key[:], value[:]) // Put into the database
	}

	// Now this is the actual test
	for i := byte(0); i < cnt; i++ {
		add()
		for i, pair := range submissions {
			require.NotNil(t, dbManager.TXCache[pair.Key], "Entry %d missing", i)
			require.Equal(t, pair.Value[:], dbManager.TXCache[pair.Key], "Entry %d has wrong value", i)
		}
	}

	dbManager.EndBatch()
	for i, pair := range submissions {
		DBValue := dbManager.DB.Get(pair.Key)
		require.NotNil(t, DBValue, "Entry %d missing", i)
		require.Equal(t, pair.Value[:], DBValue, "Entry %d has wrong value", i)
	}

}

func writeAndRead(t *testing.T, dbManager *database.Manager) {
	dbManager.AddBucket("a")
	dbManager.AddBucket("b")
	dbManager.AddBucket("c")
	d1 := []byte{1, 2, 3}
	d2 := []byte{2, 3, 4}
	d3 := []byte{3, 4, 5}
	_ = dbManager.Put("a", "", []byte("horse"), d1)
	_ = dbManager.Put("b", "", []byte("horse"), d2)
	_ = dbManager.Put("c", "", []byte("horse"), d3)
	v1 := dbManager.Get("a", "", []byte("horse"))
	v2 := dbManager.Get("b", "", []byte("horse"))
	v3 := dbManager.Get("c", "", []byte("horse"))

	if !bytes.Equal(d1, v1) || !bytes.Equal(d2, v2) || !bytes.Equal(d3, v3) {
		t.Error("All values should be equal")
	}

	if err := dbManager.PutInt64("d", "", []byte(fmt.Sprint(1)), int64(1)); err == nil {
		t.Error("Should throw an error")
	}

	for i := 0; i < 10; i++ {
		dbManager.PutBatch("a", "", common.Int64Bytes(int64(i)), []byte(fmt.Sprint(i)))
	}

	dbManager.EndBatch()

	// Sort that I can read all thousand entries
	for i := 0; i < 10; i++ {
		eKey := dbManager.GetKey("a", "", common.Int64Bytes(int64(i)))
		eValue := []byte(fmt.Sprint(i))

		fmt.Printf("key %x value %s\n", eKey, eValue)

		DBValue := dbManager.DB.Get(eKey)
		if !bytes.Equal(DBValue, eValue) {
			t.Error("failed to retrieve value ", eValue, " at: ", i, " got ", DBValue)
		}
	}

}

func TestAppID(t *testing.T) {

	dbManager := new(database.Manager)
	_ = dbManager.Init("memory", "")
	defer dbManager.Close()

	dbManager.SetAppID([]byte("one"))
	dbManager.AddBucket("a")
	dbManager.AddBucket("b")
	dbManager.AddBucket("c")
	d1 := []byte{1, 2, 3}
	d2 := []byte{2, 3, 4}
	d3 := []byte{3, 4, 5}
	_ = dbManager.Put("a", "", []byte("horse"), d1)
	_ = dbManager.Put("b", "", []byte("horse"), d2)
	_ = dbManager.Put("c", "", []byte("horse"), d3)
	v1 := dbManager.Get("a", "", []byte("horse"))
	v2 := dbManager.Get("b", "", []byte("horse"))
	v3 := dbManager.Get("c", "", []byte("horse"))

	dbManager.SetAppID([]byte("two"))
	dbManager.AddBucket("a")
	dbManager.AddBucket("b")
	dbManager.AddBucket("c")
	d4 := []byte{7, 1, 2, 3}
	d5 := []byte{8, 2, 3, 4}
	d6 := []byte{9, 3, 4, 5}
	_ = dbManager.Put("a", "", []byte("horse"), d4)
	_ = dbManager.Put("b", "", []byte("horse"), d5)
	_ = dbManager.Put("c", "", []byte("horse"), d6)
	v4 := dbManager.Get("a", "", []byte("horse"))
	v5 := dbManager.Get("b", "", []byte("horse"))
	v6 := dbManager.Get("c", "", []byte("horse"))

	if !bytes.Equal(d1, v1) || !bytes.Equal(d2, v2) || !bytes.Equal(d3, v3) ||
		!bytes.Equal(d4, v4) || !bytes.Equal(d5, v5) || !bytes.Equal(d6, v6) {
		t.Error("All values should be equal")
	}

	if bytes.Equal(v1, v4) || bytes.Equal(v2, v5) || bytes.Equal(v3, v6) {
		t.Error("all values should be different")
	}

}

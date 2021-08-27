package database_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/AccumulateNetwork/SMT/smt/storage"

	"github.com/AccumulateNetwork/SMT/smt/storage/database"
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
	}
}

func TestDBManager_TransactionsMemory(t *testing.T) {

	dbManager := new(database.Manager)
	_ = dbManager.Init("memory", "")
	writeAndRead(t, dbManager)
	dbManager.Close()
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
		if err := dbManager.PutBatch("a", "", storage.Int64Bytes(int64(i)), []byte(fmt.Sprint(i))); err != nil {
			t.Error(err)
		}
	}
	dbManager.EndBatch()

	// Sort that I can read all thousand entries
	for i := 0; i < 10; i++ {
		eKey := dbManager.GetKey("a", "", storage.Int64Bytes(int64(i)))
		eValue := []byte(fmt.Sprint(i))

		fmt.Printf("key %x value %s\n", eKey, eValue)

		DBValue := dbManager.DB.Get(eKey)
		if !bytes.Equal(DBValue, eValue) {
			t.Error("failed to retrieve value ", eValue, " at: ", i, " got ", DBValue)
		}
	}

}

func TestSalt(t *testing.T) {

	dbManager := new(database.Manager)
	_ = dbManager.Init("memory", "")
	defer dbManager.Close()

	dbManager.SetSalt([]byte("one"))
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

	dbManager.SetSalt([]byte("two"))
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

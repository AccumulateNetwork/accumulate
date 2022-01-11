package badger

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/dgraph-io/badger"
)

func TestDatabase(t *testing.T) {
	dname, e := ioutil.TempDir("", "sampledir")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dname)

	db, err := badger.Open(badger.DefaultOptions(dname))
	if err != nil {
		t.Fatal(err.Error())
	}

	defer db.Close()

	for i := 0; i < 10000; i++ {
		err = db.Update(func(txn *badger.Txn) error {
			err := txn.Set([]byte(fmt.Sprintf("answer %d", i)), []byte(fmt.Sprintf("%x this much data ", i)))
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if i%1000 == 0 {
			//	println(i)
		}
	}
	//fmt.Println("Reads")
	for i := 0; i < 10000; i++ {
		var val []byte
		err = db.View(func(txn *badger.Txn) error {
			item, err := txn.Get([]byte(fmt.Sprintf("answer %d", i)))
			if err != nil {
				t.Fatal(err)
			}
			err = item.Value(func(v []byte) error {
				val = append(val, v...)
				return nil
			})
			return nil
		})

		if string(val) != fmt.Sprintf("%x this much data ", i) {
			t.Error("Did not read data properly")
		}
	}
}

func TestDatabase2(t *testing.T) {
	dname, e := ioutil.TempDir("", "sampledir")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dname)

	db, err := badger.Open(badger.DefaultOptions(dname))
	if err != nil {
		t.Fatal(err.Error())
	}

	defer db.Close()

	txn := db.NewTransaction(true)
	for i := 0; i < 10000; i++ {
		if err := txn.Set([]byte(fmt.Sprintf("answer %d", i)), []byte(fmt.Sprintf("%x this much data ", i))); err != nil {
			t.Fatal(err)
		}
		if i%1000 == 0 {
			//	println(i)
		}
	}
	if err := txn.Commit(); err != nil {
		t.Fatal(err)
	}
	//fmt.Println("Reads")
	for i := 0; i < 10000; i++ {
		var val []byte
		err = db.View(func(txn *badger.Txn) error {
			item, err := txn.Get([]byte(fmt.Sprintf("answer %d", i)))
			if err != nil {
				t.Fatal(err)
			}
			err = item.Value(func(v []byte) error {
				val = append(val, v...)
				return nil
			})
			return nil
		})

		if string(val) != fmt.Sprintf("%x this much data ", i) {
			t.Error("Did not read data properly")
		}
	}
}

package db

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestBoltDatabase(t *testing.T) {
	dname, e := ioutil.TempDir("", "sampledir")
	if e != nil {
		t.Fatal(e)
	}

	err := os.MkdirAll(dname, 0600)
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(dname)

	filename := filepath.Join(dname, "wallet.db")

	db := BoltDB{}
	err = db.InitDB(filename)
	if err != nil {
		t.Fatal(err.Error())
	}

	defer db.Close()

	databaseTests(t, &db)
}

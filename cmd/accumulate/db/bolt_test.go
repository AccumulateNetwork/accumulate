package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBoltDatabase(t *testing.T) {
	dirName, e := os.MkdirTemp("", "boltTest")
	if e != nil {
		t.Fatal(e)
	}

	err := os.MkdirAll(dirName, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dirName)

	db := BoltDB{}
	err = db.InitDB(filepath.Join(dirName, "test.db"), "")
	//we expect it to open with a database not encrypted error
	if err != nil && err != ErrDatabaseNotEncrypted {
		t.Fatal(err.Error())
	}
	defer db.Close()

	databaseTests(t, &db)
}

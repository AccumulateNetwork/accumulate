package db

import (
	"testing"
)

func TestMemoryDatabase(t *testing.T) {
	db := MemoryDB{}
	err := db.InitDB("", "")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.Close()

	databaseTests(t, &db)
}

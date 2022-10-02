// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

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

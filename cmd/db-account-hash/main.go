// db-account-hash opens a LevelDB Accumulate database read-only and
// invokes account.Hash() for the named account. Built with -tags debug
// it prints the full decomposition (main / secondary / chains / pending)
// via the database package's DEBUG_DIDCHANGE hook.
//
// The DB must NOT be in use by another process — LevelDB requires
// exclusive access. Stop the follower (or copy the DB elsewhere) first.
package main

import (
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: db-account-hash <leveldb-path> <account-url>")
		os.Exit(1)
	}
	dbPath := os.Args[1]
	target, err := url.Parse(os.Args[2])
	must(err)

	db, err := database.OpenLevelDB(dbPath, nil)
	must(err)
	defer db.Close()

	batch := db.Begin(false)
	defer batch.Discard()

	root, err := batch.GetBptRootHash()
	must(err)
	fmt.Printf("BPT root: %x\n", root)

	acc := batch.Account(target)
	got, err := acc.Hash()
	must(err)
	fmt.Printf("account.Hash() = %x\n", got)
}

func must(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

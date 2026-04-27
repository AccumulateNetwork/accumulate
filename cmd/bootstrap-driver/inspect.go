//go:build ignore

// One-off inspector: print main-chain heights for a list of accounts.
//   go run ./cmd/bootstrap-driver/inspect.go <data-dir>
package main

import (
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: inspect <data-dir>")
		os.Exit(1)
	}
	db, err := database.OpenBadger(os.Args[1], nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	batch := db.Begin(false)
	defer batch.Discard()

	accounts := []string{
		"dn.acme",
		"dn.acme/operators",
		"dn.acme/operators/1",
		"dn.acme/network",
		"dn.acme/ledger",
		"dn.acme/anchors",
		"acme.acme",
		"bvn-apollo.acme/anchors",
		"bvn-yutu.acme/anchors",
	}
	for _, s := range accounts {
		u, _ := url.Parse(s)
		acct := batch.Account(u)
		main, err := acct.MainChain().Get()
		if err != nil {
			fmt.Printf("%-30s  main: error: %v\n", s, err)
			continue
		}
		sig, _ := acct.SignatureChain().Get()
		fmt.Printf("%-30s  main=%d  signature=%d\n", s, main.Height(),
			func() int64 {
				if sig != nil {
					return sig.Height()
				}
				return -1
			}())
	}
}

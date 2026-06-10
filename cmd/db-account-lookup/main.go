// db-account-lookup opens one or more on-disk Accumulate databases
// (badger or leveldb, read-only via TruncateBadger=true on a copy) and
// for each supplied URL reports whether the account body is present
// and prints a per-component summary. Used to determine whether the
// pre-reorg July 13 2025 backups still hold the bodies that the
// current Cyclops follower has lost.
//
// Usage:
//   db-account-lookup --db <path>[ --db <path>...] --kind {badger|leveldb} <url>...
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	bdg "gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func main() {
	var (
		dbs  stringList
		kind = flag.String("kind", "badger", "db kind: badger or leveldb")
	)
	flag.Var(&dbs, "db", "path to a database (repeatable)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: db-account-lookup --db <path>... --kind {badger|leveldb} <url>...")
		flag.PrintDefaults()
	}
	flag.Parse()
	bdg.ReadOnlyBadger = true

	if len(dbs) == 0 || flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}

	urls := make([]*url.URL, 0, flag.NArg())
	for _, s := range flag.Args() {
		u, err := url.Parse(s)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse %q: %v\n", s, err)
			os.Exit(1)
		}
		urls = append(urls, u)
	}

	for _, p := range dbs {
		fmt.Printf("\n=== %s ===\n", p)
		var (
			db  *database.Database
			err error
		)
		switch *kind {
		case "badger":
			db, err = database.OpenBadger(p, log.NewNopLogger())
		case "leveldb":
			db, err = database.OpenLevelDB(p, log.NewNopLogger())
		default:
			fmt.Fprintf(os.Stderr, "unknown kind %q\n", *kind)
			os.Exit(1)
		}
		if err != nil {
			fmt.Printf("  open error: %v\n", err)
			continue
		}

		batch := db.Begin(false)
		for _, u := range urls {
			report(batch, u)
		}
		batch.Discard()
		_ = db.Close()
	}
}

func report(batch *database.Batch, u *url.URL) {
	acc := batch.Account(u)
	main, mainErr := acc.Main().Get()
	chains, _ := acc.Chains().Get()
	dir, _ := acc.Directory().Get()
	pending, _ := acc.Pending().Get()

	stored, _ := batch.BPT().Get(acc.Key())
	storedStr := "(no BPT entry)"
	if len(stored) == 32 {
		storedStr = fmt.Sprintf("%x", stored)
	}

	switch {
	case mainErr != nil:
		fmt.Printf("  %s\n    Main: <%v>  BPT=%s  chains=%d dir=%d pending=%d\n",
			u, mainErr, storedStr, len(chains), len(dir), len(pending))
		return
	case main == nil:
		fmt.Printf("  %s\n    Main: <nil>  BPT=%s  chains=%d dir=%d pending=%d\n",
			u, storedStr, len(chains), len(dir), len(pending))
		return
	}

	type bm interface{ MarshalBinary() ([]byte, error) }
	var size int
	var sum [32]byte
	if x, ok := main.(bm); ok {
		data, _ := x.MarshalBinary()
		size = len(data)
		sum = sha256.Sum256(data)
	}
	typeStr := fmt.Sprintf("%T", main)
	if i := strings.LastIndex(typeStr, "."); i >= 0 {
		typeStr = typeStr[i+1:]
	}

	computed, hErr := acc.Hash()
	matchStr := "?"
	if hErr == nil && len(stored) == 32 {
		if string(stored) == string(computed[:]) {
			matchStr = "MATCH"
		} else {
			matchStr = "MISMATCH"
		}
	} else if hErr != nil {
		matchStr = fmt.Sprintf("hash err: %v", hErr)
	}
	fmt.Printf("  %s\n    type=%-18s Main=%dB sha256=%x BPT=%s\n    chains=%d dir=%d pending=%d  computed=%x  %s\n",
		u, typeStr, size, sum, storedStr, len(chains), len(dir), len(pending), computed, matchStr)
}

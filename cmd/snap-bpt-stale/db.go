// db-mode for snap-bpt-stale: when invoked with --db <path> instead of
// a snapshot, open the on-disk Badger or LevelDB database directly,
// walk the BPT, and for each entry compare the stored leaf to a
// freshly recomputed account.Hash().
//
// This bypasses snapshot decoding entirely (useful when the snapshot
// schema and current code are out of sync), and reads the same data
// the live node has — including any stale BPT entries that were
// committed by consensus but no longer reproduce from current state.
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	v2bpt "gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	bdg "gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func init() {
	// Allow opening Badger DBs that were not shut down cleanly. The
	// accman follower BVN data is in this state. Truncate is a one-time
	// recovery step on the COPY of the DB we operate against.
	bdg.TruncateBadger = true
}

func walkDB(path string, kind string) {
	var db *database.Database
	var err error
	switch kind {
	case "badger":
		db, err = database.OpenBadger(path, log.NewNopLogger())
	case "leveldb":
		db, err = database.OpenLevelDB(path, log.NewNopLogger())
	default:
		fmt.Fprintf(os.Stderr, "error: unknown db kind %q (want badger or leveldb)\n", kind)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", path, err)
		os.Exit(1)
	}
	defer db.Close()

	batch := db.Begin(false)
	defer batch.Discard()

	root, err := batch.GetBptRootHash()
	must(err)
	fmt.Printf("DB: %s (%s)\nBPT root: %x\n", path, kind, root)

	var (
		mismatches     []*mismatch
		examined       int
		typeCount      = map[string]int{}
		mismatchByType = map[string]int{}
	)

	must(v2bpt.ForEach(batch.BPT(), func(key *record.Key, hash []byte) error {
		examined++
		if *flagProgress && examined%10000 == 0 {
			fmt.Fprintf(os.Stderr, "  examined %d (%d mismatches so far)\n", examined, len(mismatches))
		}

		// Recover URL via the iterator (which calls getAccountUrl).
		// We use the public iterator instead of forEach to avoid
		// reaching into unexported helpers.
		_ = key
		_ = hash
		return nil
	}))

	// Use IterateAccounts which gives us *Account directly.
	examined = 0
	mismatches = nil
	for k := range typeCount {
		delete(typeCount, k)
		delete(mismatchByType, k)
	}
	it := batch.IterateAccounts()
	for it.Next() {
		examined++
		if *flagProgress && examined%10000 == 0 {
			fmt.Fprintf(os.Stderr, "  examined %d (%d mismatches so far)\n", examined, len(mismatches))
		}
		acc := it.Value()
		stored, err := batch.BPT().Get(record.NewKey("Account", acc.Url()))
		if err != nil {
			// Try the long-URL form (KeyHash + "Url").
			kh := acc.Key().Hash()
			stored, err = batch.BPT().Get(record.NewKey(kh, "Url"))
			if err != nil {
				fmt.Fprintf(os.Stderr, "  warning: BPT entry not found for %v: %v\n", acc.Url(), err)
				continue
			}
		}
		var sh [32]byte
		copy(sh[:], stored)

		main, mainErr := acc.Main().Get()
		typeStr := "<orphan>"
		if mainErr == nil && main != nil {
			typeStr = trimType(fmt.Sprintf("%T", main))
		}
		typeCount[typeStr]++

		computed, hErr := acc.Hash()
		if hErr != nil {
			mismatches = append(mismatches, &mismatch{
				url: acc.Url(), stored: sh, typeStr: typeStr,
				details: fmt.Sprintf("Hash() error: %v", hErr),
			})
			mismatchByType[typeStr]++
			continue
		}
		if computed == sh {
			continue
		}
		mismatches = append(mismatches, &mismatch{
			url: acc.Url(), stored: sh, computed: computed, typeStr: typeStr,
			details: componentSummary(acc, main),
		})
		mismatchByType[typeStr]++
		if *flagLimit > 0 && len(mismatches) >= *flagLimit {
			break
		}
	}
	must(it.Err())
	_ = sort.IntsAreSorted // tickle import in case
	report(examined, mismatches, typeCount, mismatchByType)
}

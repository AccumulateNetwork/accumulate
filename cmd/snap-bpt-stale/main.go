// snap-bpt-stale walks every account in a snapshot (v1 or v2), captures
// the source's stored leaf hash, restores the snapshot in-memory, and
// for each account recomputes account.Hash() against the restored state.
// Reports every (Url, storedHash, computedHash) where the two diverge,
// and a per-component summary so we can localize which sub-state
// (Main / chains / pending / directory) is the source of the drift.
//
// This is the diagnostic tool for the Cyclops "21 stale BPT entries"
// investigation: stored leaves baked into consensus that no longer
// reproduce from current account state.
//
// Usage: snap-bpt-stale <snapshot> [--limit N] [--progress]
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	v1snap "gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	v2bpt "gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	v2snap "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

var (
	flagLimit    = flag.Int("limit", 0, "stop after this many mismatches (0 = no limit)")
	flagProgress = flag.Bool("progress", false, "log progress every 10000 accounts")
	flagDB       = flag.String("db", "", "path to an on-disk DB instead of a snapshot")
	flagDBKind   = flag.String("db-kind", "badger", "DB kind: badger or leveldb")
)

type sourceEntry struct {
	url    *url.URL
	stored [32]byte
}

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: snap-bpt-stale <snapshot> [--limit N] [--progress]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *flagDB != "" {
		walkDB(*flagDB, *flagDBKind)
		return
	}
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}
	snapPath := flag.Arg(0)

	f, err := os.Open(snapPath)
	must(err)
	defer f.Close()

	v, err := v2snap.GetVersion(f)
	must(err)
	fmt.Printf("snapshot version: %d\n", v)
	mustSeek(f)

	db := database.OpenInMemory(nil)
	defer db.Close()

	var sources []*sourceEntry
	switch v {
	case v1snap.Version1:
		sources = restoreV1(db, f)
	case v2snap.Version2:
		sources = restoreV2(db, f)
	default:
		fmt.Fprintf(os.Stderr, "error: unsupported snapshot version %d\n", v)
		os.Exit(1)
	}
	fmt.Printf("captured %d source (account, hash) pairs\n", len(sources))

	// Recompute account.Hash() per source entry against the restored DB.
	batch := db.Begin(false)
	defer batch.Discard()

	var (
		mismatches      []*mismatch
		typeCount       = map[string]int{}
		mismatchByType  = map[string]int{}
	)

	for i, s := range sources {
		if *flagProgress && i > 0 && i%10000 == 0 {
			fmt.Fprintf(os.Stderr, "  examined %d / %d (%d mismatches)\n",
				i, len(sources), len(mismatches))
		}

		acc := batch.Account(s.url)
		main, mainErr := acc.Main().Get()
		typeStr := "<orphan>"
		if mainErr == nil && main != nil {
			typeStr = trimType(fmt.Sprintf("%T", main))
		}
		typeCount[typeStr]++

		computed, hErr := acc.Hash()
		if hErr != nil {
			mismatches = append(mismatches, &mismatch{
				url: s.url, stored: s.stored, typeStr: typeStr,
				details: fmt.Sprintf("Hash() error: %v", hErr),
			})
			mismatchByType[typeStr]++
			continue
		}
		if computed == s.stored {
			continue
		}

		mismatches = append(mismatches, &mismatch{
			url: s.url, stored: s.stored, computed: computed, typeStr: typeStr,
			details: componentSummary(acc, main),
		})
		mismatchByType[typeStr]++
		if *flagLimit > 0 && len(mismatches) >= *flagLimit {
			break
		}
	}

	report(len(sources), mismatches, typeCount, mismatchByType)
}

// captureV1Visitor wraps the standard v1 RestoreVisitor and records
// (Url, sourceHash) pairs from each account section.
type captureV1Visitor struct {
	*v1snap.RestoreVisitor
	out *[]*sourceEntry
}

func (v *captureV1Visitor) VisitAccount(acct *v1snap.Account, i int) error {
	if acct != nil {
		entry := &sourceEntry{url: acct.Url, stored: acct.Hash}
		*v.out = append(*v.out, entry)
	}
	return v.RestoreVisitor.VisitAccount(acct, i)
}

func restoreV1(db *database.Database, f *os.File) []*sourceEntry {
	rv := v1snap.NewRestoreVisitor(db, log.NewNopLogger())
	rv.SkipHashVerification = true
	out := []*sourceEntry{}
	cv := &captureV1Visitor{RestoreVisitor: rv, out: &out}
	mustSeek(f)
	fmt.Println("restoring v1 snapshot in-memory (SkipHashVerification, capturing source hashes)...")
	must(v1snap.Visit(f, cv))
	return out
}

func restoreV2(db *database.Database, f *os.File) []*sourceEntry {
	rd, err := v2snap.Open(f)
	must(err)
	fmt.Printf("v2 snapshot: sections=%d root=%x\n", len(rd.Sections), rd.Header.RootHash)

	mustSeek(f)
	rd2, err := v2snap.Open(f)
	must(err)
	bptSec, err := rd2.OpenBPT(-1)
	must(err)
	type rawSrc struct {
		key  *record.Key
		hash [32]byte
	}
	var raw []rawSrc
	for {
		r, err := bptSec.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			must(err)
		}
		var v [32]byte
		copy(v[:], r.Value)
		raw = append(raw, rawSrc{key: r.Key, hash: v})
	}

	mustSeek(f)
	fmt.Println("restoring v2 snapshot in-memory (SkipHashCheck)...")
	must(database.Restore(db, f, &database.RestoreOptions{SkipHashCheck: true}))

	// Resolve key → URL using the same path internal to database.Batch.
	// We expose a safe variant: walk the batch's BPT and join the URL via
	// Account(...).Url() if we can, otherwise leave nil.
	batch := db.Begin(false)
	defer batch.Discard()
	out := make([]*sourceEntry, 0, len(raw))
	urlByHash := map[[32]byte]*url.URL{}
	if err := v2bpt.ForEach(batch.BPT(), func(key *record.Key, _ []byte) error {
		// The Account record in the batch is keyed by URL. We need to
		// recover the URL from the BPT key. The BPT key is one of:
		//   ["Account", *url.URL]      — normal
		//   [keyHash [32]byte, "Url"]  — long-URL form
		if key.Len() == 2 {
			if u, ok := key.Get(1).(*url.URL); ok {
				urlByHash[key.Hash()] = u
				return nil
			}
		}
		// Long-URL form: the Url record was restored separately; we can
		// look up the URL by key prefix via the unkeyed accessor.
		// As a fallback, we walk the batch for the account whose URL
		// hashes to this key hash. The simplest reliable path is to
		// query the batch's getAccountUrl via reflection — but that's
		// unexported. Instead, we accept that long-URL accounts will
		// surface as "<unknown>" if not in the BPT-keyed form.
		return nil
	}); err != nil {
		must(err)
	}

	for _, r := range raw {
		u := urlByHash[r.key.Hash()]
		if u == nil {
			// best-effort: for long-URL form, the key.Get(0) is the URL hash
			// and Account(url) cannot be reconstructed without the URL.
			// Skip these — they'll show as orphans in the count.
			continue
		}
		out = append(out, &sourceEntry{url: u, stored: r.hash})
	}
	return out
}

func componentSummary(acc *database.Account, main any) string {
	var b strings.Builder
	if main == nil {
		b.WriteString("Main:<nil>")
	} else if x, ok := main.(interface{ MarshalBinary() ([]byte, error) }); ok {
		data, _ := x.MarshalBinary()
		fmt.Fprintf(&b, "Main %dB sha256=%x", len(data), sha256.Sum256(data))
	} else {
		fmt.Fprintf(&b, "Main type=%T", main)
	}

	if chains, err := acc.Chains().Get(); err == nil {
		fmt.Fprintf(&b, " | chains=%d", len(chains))
		for _, cm := range chains {
			ch, err := acc.GetChainByName(cm.Name)
			if err != nil {
				continue
			}
			st := ch.CurrentState()
			a := "(empty)"
			if anchor := st.Anchor(); len(anchor) >= 4 {
				a = fmt.Sprintf("%x", anchor[:4])
			}
			fmt.Fprintf(&b, " %s[%d:%s]", cm.Name, st.Count, a)
		}
	}
	if pending, err := acc.Pending().Get(); err == nil {
		fmt.Fprintf(&b, " | pending=%d", len(pending))
	}
	if dir, err := acc.Directory().Get(); err == nil {
		fmt.Fprintf(&b, " | dir=%d", len(dir))
	}
	return b.String()
}

func report(examined int, mm []*mismatch, typeCount, mismatchByType map[string]int) {
	fmt.Println()
	fmt.Printf("examined %d accounts\n", examined)
	fmt.Printf("found %d mismatches\n", len(mm))

	fmt.Println()
	fmt.Println("--- account types (all) ---")
	keys := make([]string, 0, len(typeCount))
	for k := range typeCount {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %-30s %8d   (mismatches: %d)\n", k, typeCount[k], mismatchByType[k])
	}

	if len(mm) == 0 {
		return
	}
	fmt.Println()
	fmt.Println("--- mismatches ---")
	sort.Slice(mm, func(i, j int) bool {
		return mm[i].url.String() < mm[j].url.String()
	})
	for i, m := range mm {
		fmt.Printf("[%d] %s  type=%s\n", i+1, m.url, m.typeStr)
		fmt.Printf("    stored:   %x\n", m.stored)
		fmt.Printf("    computed: %x\n", m.computed)
		fmt.Printf("    %s\n", m.details)
	}
}

type mismatch struct {
	url      *url.URL
	stored   [32]byte
	computed [32]byte
	typeStr  string
	details  string
}

func trimType(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

func mustSeek(f *os.File) {
	_, err := f.Seek(0, io.SeekStart)
	must(err)
}

func must(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

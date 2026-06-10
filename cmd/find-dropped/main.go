// find-dropped walks the BPT of a current follower DB, identifies
// accounts whose body (Main) is missing — orphans — and checks each
// one against the supplied pre-reorg databases to determine whether
// the body existed pre-reorg. Any orphan that HAD a body pre-reorg
// represents a state DROP since the reorg.
//
// Block-ledger orphans (acc://<bvn>.acme/ledger/<height>) are
// excluded — those are pruned routinely and don't constitute drops.
//
// Usage:
//   find-dropped --current <leveldb-path>
//                --preorg <badger-path>... (repeatable)
//                [--out <jsonl>]
//                [--include-leaf-mismatch]
//
// With --include-leaf-mismatch, also include accounts whose Main IS
// present but whose stored leaf does not match account.Hash() — i.e.
// the universe of "stale BPT" hits, which is a superset of pure
// drops.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

type result struct {
	URL              string `json:"url"`
	PostReorgOrphan  bool   `json:"post_orphan"`
	PreReorgPresent  bool   `json:"pre_present"`
	PreReorgPath     string `json:"pre_db,omitempty"`
	PreMainSha256    string `json:"pre_main_sha256,omitempty"`
	PreMainSize      int    `json:"pre_main_size,omitempty"`
	PreMainType      string `json:"pre_main_type,omitempty"`
	PostMainSha256   string `json:"post_main_sha256,omitempty"`
	PostMainSize     int    `json:"post_main_size,omitempty"`
	PostMainType     string `json:"post_main_type,omitempty"`
	StoredLeaf       string `json:"stored,omitempty"`
	ComputedLeaf     string `json:"computed,omitempty"`
	LeafMatch        bool   `json:"leaf_match,omitempty"`
}

func main() {
	var (
		current   = flag.String("current", "", "current follower LevelDB path (the BPT we walk)")
		preorgs   stringList
		outPath   = flag.String("out", "", "JSONL output path (default stdout)")
		incLeafMM = flag.Bool("include-leaf-mismatch", false, "also report accounts whose stored leaf doesn't match account.Hash() (supeset of drops)")
		progress  = flag.Int("progress", 10000, "log progress every N accounts examined")
	)
	flag.Var(&preorgs, "preorg", "pre-reorg Badger DB path (repeatable)")
	flag.Parse()
	if *current == "" || len(preorgs) == 0 {
		fmt.Fprintln(os.Stderr, "usage: find-dropped --current <leveldb> --preorg <badger>... [--out file] [--include-leaf-mismatch]")
		os.Exit(1)
	}

	bdg.ReadOnlyBadger = true

	// Open current follower (LevelDB).
	cur, err := database.OpenLevelDB(*current, log.NewNopLogger())
	must(err)
	defer cur.Close()
	curBatch := cur.Begin(false)
	defer curBatch.Discard()

	// Open all pre-reorg DBs (Badger, read-only).
	type preDB struct {
		path  string
		db    *database.Database
		batch *database.Batch
	}
	pres := make([]*preDB, 0, len(preorgs))
	for _, p := range preorgs {
		db, err := database.OpenBadger(p, log.NewNopLogger())
		if err != nil {
			fmt.Fprintf(os.Stderr, "open preorg %s: %v\n", p, err)
			os.Exit(1)
		}
		defer db.Close()
		batch := db.Begin(false)
		defer batch.Discard()
		pres = append(pres, &preDB{path: p, db: db, batch: batch})
	}

	root, err := curBatch.GetBptRootHash()
	must(err)
	fmt.Fprintf(os.Stderr, "current BPT root: %x\n", root)
	fmt.Fprintf(os.Stderr, "pre-reorg DBs:    %d\n", len(pres))

	var w *os.File
	if *outPath == "" {
		w = os.Stdout
	} else {
		w, err = os.Create(*outPath)
		must(err)
		defer w.Close()
	}
	enc := json.NewEncoder(w)

	var (
		examined        int
		orphans         int
		blockLedger     int
		dropsConfirmed  int
		dropsNoPreorg   int
		leafMismatch    int
	)

	it := curBatch.IterateAccounts()
	for it.Next() {
		examined++
		if examined%*progress == 0 {
			fmt.Fprintf(os.Stderr, "  examined=%d orphans=%d drops=%d leaf-mismatch=%d\n",
				examined, orphans, dropsConfirmed, leafMismatch)
		}
		acc := it.Value()
		u := acc.Url()
		urlStr := u.String()

		// Skip block ledger orphans.
		if isBlockLedger(urlStr) {
			if _, mainErr := acc.Main().Get(); mainErr != nil {
				blockLedger++
			}
			continue
		}

		main, mainErr := acc.Main().Get()
		isOrphan := mainErr != nil || main == nil

		// Compute leaf-match status (only for non-orphans, since
		// orphans always mismatch by construction unless leaf is the
		// empty hash).
		var stored []byte
		var computed [32]byte
		var leafMM bool
		if !isOrphan {
			stored, _ = curBatch.BPT().Get(acc.Key())
			c, err := acc.Hash()
			if err == nil {
				computed = c
				if len(stored) == 32 && string(stored) != string(computed[:]) {
					leafMM = true
					leafMismatch++
				}
			}
		} else {
			orphans++
		}

		// Skip if not orphan and not (leaf-mismatch and we want them).
		if !isOrphan && !(*incLeafMM && leafMM) {
			continue
		}

		r := &result{
			URL:             urlStr,
			PostReorgOrphan: isOrphan,
		}
		if !isOrphan {
			data, _ := mustMarshal(main)
			r.PostMainSha256 = hex.EncodeToString(sha256Sum(data))
			r.PostMainSize = len(data)
			r.PostMainType = trimType(fmt.Sprintf("%T", main))
		}
		if len(stored) == 32 {
			r.StoredLeaf = hex.EncodeToString(stored)
			r.ComputedLeaf = hex.EncodeToString(computed[:])
			r.LeafMatch = !leafMM && !isOrphan
		}

		// Look up in pre-reorg DBs.
		for _, p := range pres {
			pacc := p.batch.Account(u)
			pmain, perr := pacc.Main().Get()
			if perr == nil && pmain != nil {
				pdata, _ := mustMarshal(pmain)
				r.PreReorgPresent = true
				r.PreReorgPath = p.path
				r.PreMainSha256 = hex.EncodeToString(sha256Sum(pdata))
				r.PreMainSize = len(pdata)
				r.PreMainType = trimType(fmt.Sprintf("%T", pmain))
				break
			}
		}

		if isOrphan && r.PreReorgPresent {
			dropsConfirmed++
		} else if isOrphan {
			dropsNoPreorg++
		}

		_ = enc.Encode(r)
	}
	must(it.Err())

	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "examined %d accounts\n", examined)
	fmt.Fprintf(os.Stderr, "  orphans (Main missing, non-block-ledger): %d\n", orphans)
	fmt.Fprintf(os.Stderr, "    confirmed drops (had body pre-reorg):    %d\n", dropsConfirmed)
	fmt.Fprintf(os.Stderr, "    no pre-reorg record (created post-reorg orphan): %d\n", dropsNoPreorg)
	fmt.Fprintf(os.Stderr, "  block-ledger orphans (excluded):            %d\n", blockLedger)
	if *incLeafMM {
		fmt.Fprintf(os.Stderr, "  leaf-mismatch with body (additional):       %d\n", leafMismatch)
	}
}

func isBlockLedger(u string) bool {
	// acc://<bvn>.acme/ledger/<n>
	return strings.Contains(u, ".acme/ledger/") || strings.HasSuffix(u, ".acme/ledger")
}

func mustMarshal(v any) ([]byte, error) {
	type bm interface{ MarshalBinary() ([]byte, error) }
	if x, ok := v.(bm); ok {
		return x.MarshalBinary()
	}
	return nil, fmt.Errorf("no MarshalBinary on %T", v)
}

func sha256Sum(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

func trimType(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

var _ = url.Parse

func must(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

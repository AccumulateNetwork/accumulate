// find-silent compares the current state of every account on the
// follower BVN to its pre-reorg state, after first eliminating
// accounts that were touched by any post-reorg transaction. Anything
// that DIFFERS without a transaction explaining the change is silent
// corruption — state mutated outside the transaction system, which
// would not have been caught by single-validator "consensus" because
// there was no other validator to disagree.
//
// Inputs:
//   --current  <leveldb path>            current follower BVN
//   --preorg   <badger path>...          pre-reorg DBs (repeatable)
//   --txs      <walk-full.jsonl>         output of blockstore-walk
//   --out      <jsonl>                   per-account findings
//
// Findings:
//   * untouched + Main differs from pre-reorg
//   * untouched + Main present pre-reorg, missing now (orphan drop)
//   * untouched + Main missing pre-reorg, present now (impossible
//     under "no transaction touched it" — flagged as suspect)
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
)

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

type Finding struct {
	URL           string `json:"url"`
	Class         string `json:"class"` // "drop", "differs", "appeared", "ok"
	PostMainType  string `json:"post_type,omitempty"`
	PostMainSize  int    `json:"post_size,omitempty"`
	PostMainSha   string `json:"post_sha,omitempty"`
	PreMainType   string `json:"pre_type,omitempty"`
	PreMainSize   int    `json:"pre_size,omitempty"`
	PreMainSha    string `json:"pre_sha,omitempty"`
	PreSrc        string `json:"pre_src,omitempty"`
}

func main() {
	var (
		current  = flag.String("current", "", "current follower LevelDB")
		preorgs  stringList
		txsPath  = flag.String("txs", "", "walk-full.jsonl from blockstore-walk")
		outPath  = flag.String("out", "", "JSONL output (default stdout)")
		progress = flag.Int("progress", 10000, "log progress every N accounts")
	)
	flag.Var(&preorgs, "preorg", "pre-reorg Badger DB (repeatable)")
	flag.Parse()
	if *current == "" || len(preorgs) == 0 || *txsPath == "" {
		fmt.Fprintln(os.Stderr, "usage: find-silent --current <leveldb> --preorg <badger>... --txs walk-full.jsonl [--out file]")
		os.Exit(1)
	}
	bdg.ReadOnlyBadger = true

	// Build the touched-URL set from the blockstore walk.
	touched := loadTouched(*txsPath)
	fmt.Fprintf(os.Stderr, "touched URLs from blockstore: %d\n", len(touched))

	// Open current.
	cur, err := database.OpenLevelDB(*current, log.NewNopLogger())
	must(err)
	defer cur.Close()
	curBatch := cur.Begin(false)
	defer curBatch.Discard()

	// Open pre-reorg DBs.
	type preDB struct {
		path  string
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
		pres = append(pres, &preDB{path: p, batch: batch})
	}

	var w *os.File
	if *outPath == "" {
		w = os.Stdout
	} else {
		w, err = os.Create(*outPath)
		must(err)
		defer w.Close()
	}
	enc := json.NewEncoder(w)

	root, err := curBatch.GetBptRootHash()
	must(err)
	fmt.Fprintf(os.Stderr, "current BPT root: %x\n", root)

	var (
		examined        int
		untouched       int
		drops           int
		differs         int
		appeared        int
		preorgUnknown   int
	)

	it := curBatch.IterateAccounts()
	for it.Next() {
		examined++
		if examined%*progress == 0 {
			fmt.Fprintf(os.Stderr, "  examined=%d untouched=%d drops=%d differs=%d appeared=%d\n",
				examined, untouched, drops, differs, appeared)
		}
		acc := it.Value()
		u := acc.Url()
		urlStr := u.String()

		// Block-ledger entries are pruned routinely by design.
		if strings.Contains(urlStr, ".acme/ledger/") || strings.HasSuffix(urlStr, ".acme/ledger") {
			continue
		}

		// If touched by any post-reorg transaction, skip — those
		// state changes are explained.
		if _, ok := touched[urlStr]; ok {
			continue
		}
		untouched++

		// Look up current Main.
		main, mainErr := acc.Main().Get()
		var (
			postType string
			postSize int
			postSha  string
			postPresent bool
		)
		if mainErr == nil && main != nil {
			postPresent = true
			data, _ := mustMarshal(main)
			postType = trimType(fmt.Sprintf("%T", main))
			postSize = len(data)
			postSha = hex.EncodeToString(sha256Sum(data))
		}

		// Look up pre-reorg Main.
		var (
			prePresent bool
			preType    string
			preSize    int
			preSha     string
			preSrc     string
		)
		for _, p := range pres {
			pacc := p.batch.Account(u)
			pmain, perr := pacc.Main().Get()
			if perr == nil && pmain != nil {
				pdata, _ := mustMarshal(pmain)
				prePresent = true
				preType = trimType(fmt.Sprintf("%T", pmain))
				preSize = len(pdata)
				preSha = hex.EncodeToString(sha256Sum(pdata))
				preSrc = p.path
				break
			}
		}

		f := &Finding{
			URL:          urlStr,
			PostMainType: postType,
			PostMainSize: postSize,
			PostMainSha:  postSha,
			PreMainType:  preType,
			PreMainSize:  preSize,
			PreMainSha:   preSha,
			PreSrc:       preSrc,
		}
		switch {
		case prePresent && !postPresent:
			f.Class = "drop"
			drops++
			_ = enc.Encode(f)
		case prePresent && postPresent && preSha != postSha:
			f.Class = "differs"
			differs++
			_ = enc.Encode(f)
		case !prePresent && postPresent:
			f.Class = "appeared"
			appeared++
			_ = enc.Encode(f)
		case !prePresent && !postPresent:
			preorgUnknown++
			// neither side has a body — uninteresting, skip
		default:
			// match — skip
		}
	}
	must(it.Err())

	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "examined %d total; %d untouched-by-transactions\n", examined, untouched)
	fmt.Fprintf(os.Stderr, "  silent drops    (pre-reorg had body, current is orphan): %d\n", drops)
	fmt.Fprintf(os.Stderr, "  silent differs  (Main bytes differ pre vs current):      %d\n", differs)
	fmt.Fprintf(os.Stderr, "  silent appeared (no pre-reorg body, current has one):    %d\n", appeared)
	fmt.Fprintf(os.Stderr, "  both empty (uninteresting):                              %d\n", preorgUnknown)
}

// loadTouched extracts the set of account URLs that appear as
// principal or in `touches` of any record in the walk JSONL.
func loadTouched(path string) map[string]struct{} {
	f, err := os.Open(path)
	must(err)
	defer f.Close()
	dec := json.NewDecoder(f)
	out := make(map[string]struct{}, 8192)
	for {
		var r struct {
			Principal string   `json:"principal"`
			Touches   []string `json:"touches"`
			Signers   []string `json:"signers"`
		}
		if err := dec.Decode(&r); err != nil {
			break
		}
		if r.Principal != "" {
			out[r.Principal] = struct{}{}
		}
		for _, t := range r.Touches {
			out[t] = struct{}{}
		}
		for _, s := range r.Signers {
			out[s] = struct{}{}
			// A signer URL like acc://X/book/1 also implies activity
			// on the parent identity acc://X — credit deductions can
			// land there. Add the parent if it has more than one path
			// segment.
			if i := strings.Index(s[len("acc://"):], "/"); i >= 0 {
				out[s[:len("acc://")+i]] = struct{}{}
			}
		}
	}
	return out
}

func mustMarshal(v any) ([]byte, error) {
	type bm interface{ MarshalBinary() ([]byte, error) }
	if x, ok := v.(bm); ok {
		return x.MarshalBinary()
	}
	return nil, fmt.Errorf("no MarshalBinary on %T", v)
}

func sha256Sum(b []byte) []byte { h := sha256.Sum256(b); return h[:] }

func trimType(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

func must(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

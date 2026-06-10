// repair-account-chains evaluates each chain of an Accumulate account
// and repairs stripped completed-mark-blocks chain-by-chain, driven by
// an externally-supplied ordered entry list.
//
// Background: the 2025-07-13 reorg snapshot left some accounts' chain
// heads intact (correct Count and anchor) but stripped the stored
// per-element records and mark-point states for completed mark blocks.
// As a result Entry() fails with "cannot locate element N" for those
// indices, even though account.Hash() still matches the BPT leaf
// (account.Hash() depends only on the chain anchor, which lives in the
// head).
//
// This tool refills the missing Element / ElementIndex / States records
// WITHOUT touching the head, so account.Hash() is unchanged by
// construction. Before writing anything it replays the supplied entry
// list and requires the reconstructed head to exactly equal the stored
// head — proof that the supplied entries are this chain.
//
// Usage:
//   evaluate:  repair-account-chains --db <leveldb> --account <url>
//   repair:    repair-account-chains --db <leveldb> --account <url> \
//                  --chain main --entries <file> [--commit]
//
// The --entries file has lines of "<index> <hex32>", contiguous from 0.
// Without --commit the repair is fully validated and simulated but not
// persisted.
package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func main() {
	var (
		dbPath  = flag.String("db", "", "accumulate.db (LevelDB) path")
		acctStr = flag.String("account", "", "account URL")
		chain   = flag.String("chain", "", "chain to repair (omit to evaluate all chains)")
		entries = flag.String("entries", "", "ordered entry file for --chain (lines: <index> <hex32>)")
		commit  = flag.Bool("commit", false, "persist the repair (default: validate + simulate only)")
	)
	flag.Parse()
	if *dbPath == "" || *acctStr == "" {
		fmt.Fprintln(os.Stderr, "usage: repair-account-chains --db <path> --account <url> [--chain <name> --entries <file> [--commit]]")
		os.Exit(1)
	}

	db, err := database.OpenLevelDB(*dbPath, log.NewNopLogger())
	must(err)
	defer db.Close()

	batch := db.Begin(*chain != "")
	defer batch.Discard()

	u, err := url.Parse(*acctStr)
	must(err)
	acc := batch.Account(u)

	if *chain == "" {
		evaluateAll(batch, acc, u)
		return
	}
	if *entries == "" {
		fmt.Fprintln(os.Stderr, "--entries is required with --chain")
		os.Exit(1)
	}
	repair(batch, acc, u, *chain, *entries, *commit)
}

// evaluateAll reports the leaf/hash status of the account and the
// health of every registered chain.
func evaluateAll(batch *database.Batch, acc *database.Account, u *url.URL) {
	fmt.Printf("account : %s\n", u)
	stored, _ := batch.BPT().Get(acc.Key())
	fmt.Printf("BPT leaf: %x\n", stored)
	if h, err := acc.Hash(); err != nil {
		fmt.Printf("account.Hash(): ERROR %v\n", err)
	} else {
		fmt.Printf("account.Hash(): %x  leaf-match=%v\n", h,
			len(stored) == 32 && string(stored) == string(h[:]))
	}

	chains, err := acc.Chains().Get()
	must(err)
	fmt.Printf("\n%d chains:\n", len(chains))
	for _, cm := range chains {
		evalChain(acc, cm.Name)
	}
}

func evalChain(acc *database.Account, name string) {
	c2, err := acc.ChainByName(name)
	if err != nil {
		fmt.Printf("  %-16s ChainByName error: %v\n", name, err)
		return
	}
	mgr := c2.Inner()
	head, err := mgr.Head().Get()
	if err != nil {
		fmt.Printf("  %-16s head error: %v\n", name, err)
		return
	}
	count := head.Count
	markFreq := mgr.MarkFreq()

	var firstBad, lastBad int64 = -1, -1
	bad := 0
	for i := int64(0); i < count; i++ {
		if _, err := mgr.Entry(i); err != nil {
			bad++
			if firstBad < 0 {
				firstBad = i
			}
			lastBad = i
		}
	}
	var missingMarks []uint64
	for mp := markFreq - 1; mp < count; mp += markFreq {
		if _, err := mgr.States(uint64(mp)).Get(); err != nil {
			missingMarks = append(missingMarks, uint64(mp))
		}
	}

	status := "intact"
	if bad > 0 {
		status = fmt.Sprintf("HOLE: %d unreadable (indices %d..%d)", bad, firstBad, lastBad)
	}
	fmt.Printf("  %-16s count=%-6d markFreq=%-5d %s", name, count, markFreq, status)
	if len(missingMarks) > 0 {
		fmt.Printf("  missing-mark-states=%v", missingMarks)
	}
	fmt.Println()
}

// repair refills the stripped records of one chain from the supplied
// ordered entry list, after proving the entries reconstruct the chain.
func repair(batch *database.Batch, acc *database.Account, u *url.URL, name, entriesPath string, commit bool) {
	entries := loadEntries(entriesPath)
	fmt.Printf("account: %s   chain: %s\n", u, name)
	fmt.Printf("supplied entries: %d\n", len(entries))

	c2, err := acc.ChainByName(name)
	must(err)
	mgr := c2.Inner()
	storedHead, err := mgr.Head().Get()
	must(err)
	markFreq := mgr.MarkFreq()
	fmt.Printf("stored head: count=%d markFreq=%d anchor=%x\n",
		storedHead.Count, markFreq, storedHead.Anchor())

	if int64(len(entries)) != storedHead.Count {
		fail("supplied %d entries but chain head count is %d — refusing to proceed",
			len(entries), storedHead.Count)
	}

	leafBefore, _ := batch.BPT().Get(acc.Key())
	hashBefore, err := acc.Hash()
	must(err)

	// Replay the supplied entries -> reconstruct head + mark-point states.
	rHead, rStates := replay(entries, markFreq)

	// SAFETY GATE: the replayed head must exactly equal the stored head.
	// If it does, the supplied entries provably ARE this chain and the
	// refill cannot change the anchor / account.Hash().
	if !rHead.Equal(storedHead) {
		fmt.Fprintf(os.Stderr, "  stored head: count=%d anchor=%x\n", storedHead.Count, storedHead.Anchor())
		fmt.Fprintf(os.Stderr, "  replay head: count=%d anchor=%x\n", rHead.Count, rHead.Anchor())
		fail("replayed head does not match stored head — supplied entries are not this chain")
	}
	fmt.Printf("validation: replayed head EXACTLY matches stored head (anchor %x)\n", rHead.Anchor())

	// Apply only the missing records.
	var putElem, putIdx, putState int
	for i, h := range entries {
		if _, err := mgr.Element(uint64(i)).Get(); errors.Is(err, errors.NotFound) {
			must(mgr.Element(uint64(i)).Put(copyb(h)))
			putElem++
		}
		if _, err := mgr.ElementIndex(h).Get(); errors.Is(err, errors.NotFound) {
			must(mgr.ElementIndex(h).Put(uint64(i)))
			putIdx++
		}
	}
	for mp, st := range rStates {
		if _, err := mgr.States(mp).Get(); errors.Is(err, errors.NotFound) {
			must(mgr.States(mp).Put(st))
			putState++
		}
	}
	fmt.Printf("refilled: %d element records, %d element-index entries, %d mark-point states\n",
		putElem, putIdx, putState)

	// VERIFY: every entry readable; head + account.Hash() unchanged.
	badAfter := 0
	for i := range entries {
		if _, err := mgr.Entry(int64(i)); err != nil {
			badAfter++
		}
	}
	headAfter, err := mgr.Head().Get()
	must(err)
	hashAfter, err := acc.Hash()
	must(err)

	headOK := headAfter.Equal(storedHead)
	hashOK := hashBefore == hashAfter
	leafOK := len(leafBefore) == 32 && string(leafBefore) == string(hashAfter[:])
	fmt.Printf("verify: unreadable entries after repair = %d\n", badAfter)
	fmt.Printf("verify: head unchanged                  = %v\n", headOK)
	fmt.Printf("verify: account.Hash() unchanged         = %v (%x -> %x)\n",
		hashOK, hashBefore[:8], hashAfter[:8])
	fmt.Printf("verify: account.Hash() matches BPT leaf  = %v\n", leafOK)

	if badAfter != 0 || !headOK || !hashOK || !leafOK {
		fail("post-repair verification failed — not committing")
	}

	if commit {
		must(batch.Commit())
		fmt.Println("\nCOMMITTED — chain repaired.")
	} else {
		fmt.Println("\nDRY RUN OK — all checks passed. Re-run with --commit to persist.")
	}
}

// replay reconstructs the chain head and mark-point states from an
// ordered entry list, mirroring merkle.Chain.AddEntry exactly.
func replay(entries [][]byte, markFreq int64) (*merkle.State, map[uint64]*merkle.State) {
	head := new(merkle.State)
	states := map[uint64]*merkle.State{}
	markMask := markFreq - 1
	for _, h := range entries {
		switch (head.Count + 1) & markMask {
		case 0: // last element of a mark block
			head.AddEntry(copyb(h))
			states[uint64(head.Count)-1] = head.Copy()
		case 1: // first element of a new mark block
			head.HashList = head.HashList[:0]
			head.AddEntry(copyb(h))
		default:
			head.AddEntry(copyb(h))
		}
	}
	return head, states
}

func loadEntries(path string) [][]byte {
	f, err := os.Open(path)
	must(err)
	defer f.Close()
	m := map[int64][]byte{}
	maxIdx := int64(-1)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		p := strings.Fields(sc.Text())
		if len(p) != 2 {
			continue
		}
		idx, err := strconv.ParseInt(p[0], 10, 64)
		must(err)
		b, err := hex.DecodeString(p[1])
		must(err)
		if len(b) != 32 {
			fail("entry %d is %d bytes, expected 32", idx, len(b))
		}
		m[idx] = b
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	out := make([][]byte, 0, maxIdx+1)
	for i := int64(0); i <= maxIdx; i++ {
		b, ok := m[i]
		if !ok {
			fail("entries file is missing index %d (must be contiguous from 0)", i)
		}
		out = append(out, b)
	}
	return out
}

func copyb(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ABORT: "+format+"\n", args...)
	os.Exit(1)
}

// snap-decompose restores a snapshot v2 file into an in-memory DB and
// then, for the specified account, prints the source's stored BPT leaf
// hash, the restored account's recomputed account.Hash(), and the
// per-component contributions (hashState/hashSecondaryState/hashChains/
// hashPending) so we can identify which component fails to reproduce.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	psnap "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: snap-decompose <snapshot> <account-url>")
		os.Exit(1)
	}
	snapPath := os.Args[1]
	target, err := url.Parse(os.Args[2])
	must(err)

	f, err := os.Open(snapPath)
	must(err)
	defer f.Close()

	rd, err := psnap.Open(f)
	must(err)
	fmt.Printf("snapshot: version=%d sections=%d root=%x\n",
		rd.Header.Version, len(rd.Sections), rd.Header.RootHash[:8])

	// Read the BPT section to find the source's stored leaf for target.
	if _, err := f.Seek(0, 0); err != nil {
		must(err)
	}
	rd2, err := psnap.Open(f)
	must(err)
	bpt, err := rd2.OpenBPT(-1)
	must(err)

	// We'll match the target account to its BPT key hash AFTER restore by
	// asking the restored DB for the account's key hash, then looking it
	// up in the storedLeaf map below.
	storedLeaf := map[[32]byte][32]byte{}
	for {
		r, err := bpt.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			must(err)
		}
		var v [32]byte
		copy(v[:], r.Value)
		storedLeaf[r.Key.Hash()] = v
	}
	fmt.Printf("snapshot has %d BPT entries\n", len(storedLeaf))

	// Restore with SkipHashCheck so the per-account check doesn't abort.
	if _, err := f.Seek(0, 0); err != nil {
		must(err)
	}
	db := database.OpenInMemory(nil)
	defer db.Close()
	fmt.Println("restoring with SkipHashCheck...")
	must(database.Restore(db, f, &database.RestoreOptions{SkipHashCheck: true}))

	batch := db.Begin(false)
	defer batch.Discard()

	rootAfter, err := batch.GetBptRootHash()
	must(err)
	fmt.Printf("BPT root after restore: %x  (source: %x)\n",
		rootAfter[:8], rd.Header.RootHash[:8])

	// Find the target account in the restored DB.
	acc := batch.Account(target)
	keyHash := acc.Key().Hash()
	stored, ok := storedLeaf[keyHash]
	if !ok {
		fmt.Printf("account %s NOT in source BPT (keyHash=%x)\n", target, keyHash[:8])
		os.Exit(2)
	}
	fmt.Printf("source stored leaf for %s: %x\n", target, stored)

	// Recompute account.Hash() over the restored state.
	got, err := acc.Hash()
	must(err)
	fmt.Printf("restored account.Hash():     %x\n", got)
	if got == stored {
		fmt.Println("MATCH — this account would PASS the per-account check.")
		return
	}
	fmt.Println("MISMATCH — drilling into components.")

	// account.Hash() is composed via observer.hashState() of:
	//   - Main             (a single value)
	//   - SecondaryState   (Directory + ledger Events.BPT)
	//   - Chains           (per chain Anchor)
	//   - Pending          (V1 + V2 pending records)
	//
	// We don't have direct access to these from outside the database
	// package; print Main bytes + chain Anchors so we can compare against
	// the running follower's same structures (the source has the live DB
	// so a parallel `accumulated debug` invocation can read both).

	main, err := acc.Main().Get()
	if err == nil {
		b, _ := main.MarshalBinary()
		fmt.Printf("Main (type=%T) %d bytes  sha256=%x\n",
			main, len(b), sha256.Sum256(b))
	} else {
		fmt.Printf("Main: %v\n", err)
	}

	// Print chain heads + anchors for every chain on the account.
	chains, err := acc.Chains().Get()
	must(err)
	fmt.Printf("chains (%d):\n", len(chains))
	for _, cm := range chains {
		ch, err := acc.GetChainByName(cm.Name)
		if err != nil {
			fmt.Printf("  %-25s ERROR %v\n", cm.Name, err)
			continue
		}
		st := ch.CurrentState()
		anchor := st.Anchor()
		hashListLen := len(st.HashList)
		pendingLen := len(st.Pending)
		anchorStr := "(empty)"
		if len(anchor) >= 8 {
			anchorStr = fmt.Sprintf("%x", anchor[:8])
		} else if len(anchor) > 0 {
			anchorStr = fmt.Sprintf("%x", anchor)
		}
		fmt.Printf("  %-25s type=%-12s count=%-6d hashList=%d pending=%d  anchor=%s\n",
			cm.Name, cm.Type, st.Count, hashListLen, pendingLen, anchorStr)
	}

	// Pending list (V1 + V2).
	pending, err := acc.Pending().Get()
	if err != nil && !errors.Is(err, errors.NotFound) {
		fmt.Printf("Pending error: %v\n", err)
	} else {
		fmt.Printf("pending: %d txids\n", len(pending))
		for i, txid := range pending {
			fmt.Printf("  [%d] %s\n", i, txid)
		}
	}

	// Directory (children of the account, contributes to SecondaryState).
	dir, err := acc.Directory().Get()
	if err != nil && !errors.Is(err, errors.NotFound) {
		fmt.Printf("Directory error: %v\n", err)
	} else {
		fmt.Printf("directory: %d entries\n", len(dir))
	}
}

func must(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func _hex(b []byte) string { return hex.EncodeToString(b) }

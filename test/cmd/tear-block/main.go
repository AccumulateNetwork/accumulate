// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Command tear-block tears one stored block of a STOPPED node's state tree, the
// way run 20261004T195558Z found one torn: the block's own record goes back to
// an older version the node retained, while the root's block keeps the hash of
// the current one. It exists to test the repair (executor spec, "Sync", §3b,
// #4523) on a real store. It corrupts the store it is given. Never point it at
// a node anyone depends on.
//
//	tear-block -data-dir /root/.accumulate/bvn1-5 -partition BVN1 [-prefix 34] [-dry-run]
package main

import (
	"bytes"
	"context"
	"encoding/hex"
	stderrors "errors"
	"flag"
	"fmt"
	"os"

	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func main() {
	dataDir := flag.String("data-dir", "", "the stopped node's data directory (holding accumulate.toml)")
	partition := flag.String("partition", "", "the partition whose store to tear, e.g. BVN1")
	prefixHex := flag.String("prefix", "", "the stored block to tear, as one hex byte; empty picks the first block with a retained older version")
	dryRun := flag.Bool("dry-run", false, "say what would be torn, and write nothing")
	flag.Parse()
	if *dataDir == "" || *partition == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := tear(*dataDir, *partition, *prefixHex, *dryRun); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func tear(dataDir, partition, prefixHex string, dryRun bool) error {
	store, _, closeStore, err := run.OpenPartitionStore(context.Background(), dataDir, partition)
	if err != nil {
		return err
	}
	defer func() { _ = closeStore() }()
	db := database.New(store, nil)

	read := db.Begin(false)
	defer read.Discard()
	tree := read.BPT()
	earliest, ok, err := tree.EarliestRetained()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("the store retains no BPT history, so there is no older version to tear a block back to")
	}

	var candidates []byte
	if prefixHex != "" {
		b, err := hex.DecodeString(prefixHex)
		if err != nil || len(b) != 1 {
			return fmt.Errorf("prefix %q is not one hex byte", prefixHex)
		}
		candidates = b
	} else {
		root, err := tree.GetStoredBlock(nil)
		if err != nil {
			return fmt.Errorf("read the root's block: %w", err)
		}
		for _, s := range root.Slots {
			if s.Branch {
				candidates = append(candidates, s.Index)
			}
		}
	}

	for _, p := range candidates {
		key, err := tree.StoredBlockKey([]byte{p})
		if err != nil {
			return err
		}
		nodeKey, ok := key.Get(key.Len() - 1).([32]byte)
		if !ok {
			return fmt.Errorf("the record key of the block under %x has no node key", p)
		}
		current, err := readRaw(store, key)
		if err != nil {
			return err
		}
		old, err := olderVersion(tree, nodeKey, earliest, current)
		if err != nil {
			return err
		}
		if old == nil {
			if prefixHex != "" {
				return fmt.Errorf("the block under %x has no retained version older than its current one", p)
			}
			continue
		}

		fmt.Printf("tearing the block under %02x: its record goes back to a retained version (%d bytes, current %d)\n", p, len(old), len(current))
		if dryRun {
			return nil
		}
		w := store.Begin(nil, true)
		if err := w.Put(key, old); err != nil {
			w.Discard()
			return err
		}
		if err := w.Commit(); err != nil {
			return err
		}

		check := db.Begin(false)
		_, err = check.BPT().GetStoredBlock([]byte{p})
		check.Discard()
		var bad *bpt.InconsistentBlockError
		if !stderrors.As(err, &bad) {
			return fmt.Errorf("wrote the old version, but the block under %x does not fail its check (%v)", p, err)
		}
		fmt.Println("torn:", bad)
		return nil
	}
	return fmt.Errorf("no stored block has a retained version older than its current one")
}

// olderVersion is a retained version of the node that differs from current,
// or nil.
func olderVersion(tree *bpt.BPT, nodeKey [32]byte, earliest uint64, current []byte) ([]byte, error) {
	data, ok, err := tree.NodeAt(nodeKey, earliest)
	if err != nil || !ok {
		return nil, err
	}
	if bytes.Equal(data, current) {
		return nil, nil
	}
	return data, nil
}

func readRaw(store keyvalue.Beginner, key *record.Key) ([]byte, error) {
	r := store.Begin(nil, false)
	defer r.Discard()
	return r.Get(key)
}

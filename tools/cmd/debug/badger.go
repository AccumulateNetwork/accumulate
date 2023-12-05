// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/dgraph-io/badger"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
)

func init() {
	badgerTruncateCmd.Run = truncate
	badgerCompactCmd.Run = compact

	cmd.AddCommand(badgerCmd)
	badgerCmd.AddCommand(
		badgerTruncateCmd,
		badgerCompactCmd,
		badgerCloneCmd,
		badgerLsCmd,
	)
}

var badgerCmd = &cobra.Command{
	Use:   "badger",
	Short: "Badger maintenance utilities",
}

var badgerTruncateCmd = &cobra.Command{
	Use:   "truncate [database]",
	Short: "Truncate the value log of a corrupted database",
	Args:  cobra.ExactArgs(1),
}

var badgerCompactCmd = &cobra.Command{
	Use:   "compact [database]",
	Short: "Compact the database",
	Args:  cobra.ExactArgs(1),
}

var badgerCloneCmd = &cobra.Command{
	Use:   "clone [source] [destination]",
	Short: "Clone the database",
	Args:  cobra.ExactArgs(2),
	Run:   func(_ *cobra.Command, args []string) { badgerClone(args[0], args[1]) },
}

var badgerLsCmd = &cobra.Command{
	Use:  "ls [database]",
	Args: cobra.ExactArgs(1),
	Run:  func(_ *cobra.Command, args []string) { badgerLs(args[0]) },
}

var badgerCompactUseGC = badgerCompactCmd.Flags().Bool("use-gc", false, "Use Badger's GC instead of cloning the database")

func truncate(_ *cobra.Command, args []string) {
	opt := badger.DefaultOptions(args[0]).
		WithTruncate(true).
		WithNumVersionsToKeep(1)
	db, err := badger.Open(opt)
	if err != nil {
		log.Fatalf("error opening badger: %v", err)
	}
	defer db.Close()
}

func compact(_ *cobra.Command, args []string) {
	if *badgerCompactUseGC {
		// Based on https://github.com/dgraph-io/badger/issues/718
		// Credit to https://github.com/mschoch

		db, err := badger.Open(
			badger.DefaultOptions(args[0]).
				WithTruncate(true))
		check(err)
		defer db.Close()

		count := 0
		for err == nil {
			log.Printf("starting value log gc")
			count++
			err = db.RunValueLogGC(0.5)
		}
		if err == badger.ErrNoRewrite {
			log.Printf("no rewrite needed")
		} else if err != nil {
			log.Fatalf("error running value log gc: %v", err)
		}
		log.Printf("completed gc, ran %d times", count)
		return
	}

	// Badger's compaction tools do not work. So instead we're just going to
	// create a new database, copy the latest version of each value, and swap.
	srcPath := args[0]
	dstPath := srcPath + ".tmp"
	badgerClone(srcPath, dstPath)

	// Move source to a backup location
	bak := srcPath + ".bak"
	check(os.Rename(srcPath, bak))

	// Move destination to source
	if err := os.Rename(dstPath, srcPath); err != nil {
		// Revert
		log.Printf("Failed to replace %v, reverting", srcPath)
		check(os.Rename(bak, srcPath))
	}

	// Delete the backup
	check(os.RemoveAll(bak))
}

func badgerClone(srcPath, dstPath string) {
	srcDb, err := badger.Open(badger.DefaultOptions(srcPath))
	check(err)
	defer func() { check(srcDb.Close()) }()

	dstDb, err := badger.Open(badger.DefaultOptions(dstPath))
	check(err)
	defer func() { check(dstDb.Close()) }()

	dstwr := dstDb.NewWriteBatch()
	defer func() { check(dstwr.Flush()) }()

	srctx := srcDb.NewTransaction(false)
	defer srctx.Discard()
	it := srctx.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()

	for it.Seek(nil); it.Valid(); it.Next() {
		x := it.Item()
		key := x.KeyCopy(nil)
		val, err := x.ValueCopy(nil)
		check(err)
		check(dstwr.Set(key, val))
	}
}

func badgerLs(dbPath string) {
	opt := badger.DefaultOptions(dbPath).
		WithNumCompactors(0).
		WithTruncate(true)
	db, err := badger.Open(opt)
	if err != nil {
		log.Fatalf("error opening badger: %v", err)
	}
	defer db.Close()

	tx := db.NewTransaction(false)
	itopts := badger.DefaultIteratorOptions
	itopts.PrefetchValues = false
	itopts.AllVersions = true
	it := tx.NewIterator(itopts)
	defer it.Close()

	n := new(node)
	for it.Seek(nil); it.Valid(); it.Next() {
		x := it.Item()
		k := new(record.Key)
		if k.UnmarshalBinary(x.Key()) != nil {
			fatalf("cannot unmarshal key: is this database using compressed keys?")
		}
		n.Add(k, x.EstimatedSize())
	}
	n.Print()
}

type node struct {
	prefix   string
	children map[string]*node
	size     int64
	depth    int
	versions int
}

func (n *node) get(s *record.Key) *node {
	if n.depth >= s.Len() {
		return n
	}

	if n.children == nil {
		n.children = map[string]*node{}
	}
	ss := s.SliceI(n.depth).SliceJ(1).String()
	m, ok := n.children[ss]
	if ok {
		return m
	}

	m = &node{depth: n.depth + 1, prefix: s.SliceJ(n.depth + 1).String()}
	n.children[ss] = m
	return m
}

func (n *node) Versions(s *record.Key) int {
	if n.depth >= s.Len() {
		return n.versions
	}
	return n.get(s).Versions(s)
}

func (n *node) Add(s *record.Key, z int64) {
	n.size += z
	if n.depth >= s.Len() {
		n.versions++
		return
	}

	n.get(s).Add(s, z)
}

func (n *node) Print() {
	if n.children == nil {
		return
	}

	if n.prefix == "Message" {
		fmt.Printf("%v   %v\n", byteCountIEC(n.size), n.prefix)
		return
	}

	var keys []string
	for k := range n.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		n.children[k].Print()
	}
	fmt.Printf("%v   %v   %d\n", byteCountIEC(n.size), n.prefix, n.versions)
}

func byteCountIEC(b int64) string {
	const unit = 1024
	if b < unit {
		s := fmt.Sprintf("%d", b)
		s = strings.Repeat(" ", 4-len(s)) + s
		return fmt.Sprintf("%v   B", s)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= 9999; n /= unit {
		div *= unit
		exp++
	}
	s := fmt.Sprintf("%.0f", float64(b)/float64(div))
	s = strings.Repeat(" ", 4-len(s)) + s
	return fmt.Sprintf("%s %ciB", s, "KMGTPE"[exp])
}

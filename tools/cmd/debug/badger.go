// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"log"
	"runtime"

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
		badgerFlattenCmd,
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

var badgerFlattenCmd = &cobra.Command{
	Use:   "flatten [database]",
	Short: "Flatten Badger's LSM",
	Args:  cobra.ExactArgs(1),
	Run:   badgerFlatten,
}

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
	} else {
		log.Fatalf("error running value log gc: %v", err)
	}
	log.Printf("completed gc, ran %d times", count)
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

func badgerFlatten(_ *cobra.Command, args []string) {
	db, err := badger.Open(badger.DefaultOptions(args[0]).
		WithTruncate(true).
		WithNumCompactors(0))
	check(err)
	defer db.Close()

	check(db.Flatten(runtime.NumCPU()))
}

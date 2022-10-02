// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"log"

	"github.com/dgraph-io/badger"
	"github.com/spf13/cobra"
)

func init() {
	badgerTruncateCmd.Run = truncate
	badgerCompactCmd.Run = compact

	cmd.AddCommand(badgerCmd)
	badgerCmd.AddCommand(badgerTruncateCmd, badgerCompactCmd)
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
	Short: "Run GC on a database to compact it",
	Args:  cobra.ExactArgs(1),
}

var compactRatio = badgerCompactCmd.Flags().Float64("ratio", 0.5, "Badger GC ratio")

func truncate(_ *cobra.Command, args []string) {
	opt := badger.DefaultOptions(args[0]).
		WithTruncate(true)
	db, err := badger.Open(opt)
	if err != nil {
		log.Fatalf("error opening badger: %v", err)
	}
	defer db.Close()
}

func compact(_ *cobra.Command, args []string) {
	// Based on https://github.com/dgraph-io/badger/issues/718
	// Credit to https://github.com/mschoch

	opt := badger.DefaultOptions(args[0])
	db, err := badger.Open(opt)
	if err != nil {
		log.Fatalf("error opening badger: %v", err)
	}
	defer db.Close()

	count := 0
	for err == nil {
		log.Printf("starting value log gc")
		count++
		err = db.RunValueLogGC(*compactRatio)
	}
	if err == badger.ErrNoRewrite {
		log.Printf("no rewrite needed")
	} else if err != nil {
		log.Fatalf("error running value log gc: %v", err)
	}
	log.Printf("completed gc, ran %d times", count)
}

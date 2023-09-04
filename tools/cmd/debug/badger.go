// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	tmlog "github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	database2 "gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"log"
	"os"

	"github.com/dgraph-io/badger"
	"github.com/spf13/cobra"
)

func init() {
	badgerTruncateCmd.Run = truncate
	badgerCompactCmd.Run = compact

	cmd.AddCommand(badgerCmd)
	badgerCmd.AddCommand(badgerTruncateCmd, badgerCompactCmd, badgerWalkCmd)
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

var badgerWalkCmd = &cobra.Command{
	Use:   "walk [database] [account url] [\"main\" | \"signature\" | \"pending\"]",
	Short: "Walk the database and print out the chain for a given url",
	Args:  cobra.ExactArgs(3),
	Run:   walk,
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

func walkCallback(rec database2.Record) (skip bool, err error) {
	fmt.Printf("%s\n", rec.Key().String())
	return false, nil
}
func walk(_ *cobra.Command, args []string) {
	logger := tmlog.NewTMLogger(os.Stderr)
	db, err := database.OpenBadger(args[0], logger)
	check(err)
	defer db.Close()

	batch := db.Begin(false)
	defer batch.Discard()

	u, err := url.Parse(args[1])
	check(err)
	record := batch.Account(u)
	chains, err := record.Chains().Get()
	check(err)

	r := new(api.RecordRange[*api.ChainRecord])
	r.Total = uint64(len(chains))
	r.Records = make([]*api.ChainRecord, len(chains))
	for _, c := range chains {
		log.Printf("--> Chain %s of type %s", c.Name, c.Type)
		chain, err := record.ChainByName(c.Name)
		check(err)

		for i := int64(0); ; i++ {
			entry, err := chain.Entry(i)
			if err != nil {
				break
			}
			log.Printf("Entry %d: %x", i, entry)
		}

		//cr, err := s.queryChainByName(ctx, record, c.Name)
		//if err != nil {
		//	return nil, errors.UnknownError.WithFormat("chain %s: %w", c.Name, err)
		//}

		//r.Records[i] = cr
	}

	//	var msg messaging.MessageWithTransaction

	//values := batch.Account(u).RootChain().Get() //.Main()

	//values := batch.Message()
	//opts := database2.WalkOptions{}
	//opts.Values = true
	//opts.IgnoreIndices = false
	//values.Walk(opts, walkCallback)

	//log.Printf("key: %s\n", values.Key().String())
	//for i, v := range values. {
	//
	//}
	//if err != nil {
	//	return nil, errors.UnknownError.WithFormat("load system ledger: %w", err)
	//}

}

package main

import (
	"log"

	"github.com/dgraph-io/badger"
	"github.com/spf13/cobra"
)

func main() {
	cmd.Run = func(cmd *cobra.Command, _ []string) { _ = cmd.Usage() }
	cmdTruncate.Run = truncate
	cmdCompact.Run = compact

	cmd.AddCommand(cmdTruncate, cmdCompact)
	_ = cmd.Execute()
}

var cmd = &cobra.Command{
	Use:   "badger",
	Short: "Badger maintenance utilities",
}

var cmdTruncate = &cobra.Command{
	Use:   "truncate [database]",
	Short: "Truncate the value log of a corrupted database",
	Args:  cobra.ExactArgs(1),
}

var cmdCompact = &cobra.Command{
	Use:   "compact [database]",
	Short: "Run GC on a database to compact it",
	Args:  cobra.ExactArgs(1),
}

var compactRatio = cmdCompact.Flags().Float64("ratio", 0.5, "Badger GC ratio")

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

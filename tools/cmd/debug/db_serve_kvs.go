// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"net"

	"github.com/spf13/cobra"
	cmdutil "gitlab.com/accumulatenetwork/accumulate/internal/util/cmd"
)

var cmdDbServe = &cobra.Command{
	Use:   "serve [database] [address]",
	Short: "Serve a database",
	Example: `` +
		`  debug db serve badger:///path/to/source.db unix:///path/to/serve.sock` + "\n",
	Args: cobra.ExactArgs(2),
	Run:  serveDatabasesCmd,
}

func init() {
	cmdDb.AddCommand(cmdDbServe)
}

func serveDatabasesCmd(_ *cobra.Command, args []string) {
	srcDb, _ := openDbUrl(args[0], false)
	_, dstAddr := openDbUrl(args[1], true)

	if srcDb == nil {
		fatalf("first argument must be a local database")
	}
	if dstAddr == nil {
		fatalf("second argument must be a listening address")
	}

	serveDatabases(srcDb, dstAddr)
}

func serveDatabases(db dbCloser, addr net.Addr) {
	defer func() { check(db.Close()) }()
	ctx := cmdutil.ContextForMainProcess(context.Background())
	check(serveKeyValueStore(ctx, db, addr))
}

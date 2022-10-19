// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/rpc/client/http"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

func init() {
	cmdMain.AddCommand(cmdSync, cmdRestoreSnapshot)
	cmdSync.AddCommand(cmdSyncSnapshot)
}

var cmdSync = &cobra.Command{
	Use:   "sync",
	Short: "Configure synchronization",
}

var cmdSyncSnapshot = &cobra.Command{
	Use:   "snapshot [tendermint RPC server] [height] [hash]",
	Short: "Configures state sync to pull a snapshot",
	Args:  cobra.ExactArgs(3),
	Run:   syncToSnapshot,
}

var cmdRestoreSnapshot = &cobra.Command{
	Use:   "restore-snapshot [file]",
	Short: "Rebuild the accumulate database from a snapshot",
	Args:  cobra.ExactArgs(1),
	Run:   restoreSnapshot,
}

func syncToSnapshot(_ *cobra.Command, args []string) {
	client, err := http.New(args[0], args[0]+"/websocket")
	checkf(err, "server")

	height, err := strconv.ParseInt(args[1], 10, 64)
	checkf(err, "height")

	hash, err := hex.DecodeString(args[2])
	checkf(err, "hash")

	c, err := config.Load(flagMain.WorkDir)
	checkf(err, "load configuration")

	// The Tendermint height is one more than the last commit height
	blockHeight := height + 1

	// TODO The user should specify the block header hash directly instead of us
	// querying a node for it
	tmblock, err := client.Block(context.Background(), &blockHeight)
	checkf(err, "fetching block")

	if tmblock == nil {
		fatalf("Block not found")
	}

	if tmblock.Block.LastCommit.Height != height {
		fatalf("Last commit height does not match: want %d, got %d", height, tmblock.Block.Height)
	}

	if !bytes.Equal(tmblock.Block.AppHash, hash) {
		fatalf("App hash does not match: want %x, got %x", hash, tmblock.Block.AppHash)
	}

	ss := c.StateSync
	ss.Enable = true
	// ss.UseP2P = true
	ss.TrustHeight = tmblock.Block.Height
	ss.TrustHash = tmblock.Block.Header.Hash().String()

	err = config.Store(c)
	checkf(err, "store configuration")
}

func restoreSnapshot(_ *cobra.Command, args []string) {
	f, err := os.Open(args[0])
	checkf(err, "snapshot file")
	defer f.Close()

	daemon, err := accumulated.Load(flagMain.WorkDir, func(c *config.Config) (io.Writer, error) {
		return logging.NewConsoleWriter(c.LogFormat)
	})
	checkf(err, "load daemon")

	err = daemon.LoadSnapshot(f)
	checkf(err, "load snapshot")
}

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/rpc/client/http"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
)

func init() {
	cmdMain.AddCommand(cmdSync)
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

func syncToSnapshot(_ *cobra.Command, args []string) {
	client, err := http.New(args[0])
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
	ss.UseP2P = true
	ss.TrustHeight = tmblock.Block.Height
	ss.TrustHash = tmblock.Block.Header.Hash().String()

	err = config.Store(c)
	checkf(err, "store configuration")
}

package main

import (
	"encoding/hex"
	"strconv"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/config"
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
	Use:   "snapshot [height] [hash]",
	Short: "Configures state sync to pull a snapshot",
	Args:  cobra.ExactArgs(2),
	Run:   syncToSnapshot,
}

func syncToSnapshot(_ *cobra.Command, args []string) {
	height, err := strconv.ParseInt(args[0], 10, 64)
	checkf(err, "parse height")

	_, err = hex.DecodeString(args[1])
	checkf(err, "check hash")

	c, err := config.Load(flagMain.WorkDir)
	checkf(err, "load configuration")

	ss := c.StateSync
	ss.Enable = true
	ss.UseP2P = true
	ss.TrustHeight = height
	ss.TrustHash = args[1]

	err = config.Store(c)
	checkf(err, "store configuration")
}

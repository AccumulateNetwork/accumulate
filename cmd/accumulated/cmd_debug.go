// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdDebug = &cobra.Command{
	Use:   "debug",
	Short: "Operator tools for a node's store",
}

var cmdDebugRepair = &cobra.Command{
	Use:   "repair <data-dir>",
	Short: "Repair, with the node stopped, the failures its repair log records",
	Long: `Reads the partition's repair log (consensus/<partition>/repairs.jsonl under
the node's data directory), and repairs each failure the node recorded and did
not repair: a stored block of its state tree that fails its own check has the
sections under it taken again from the peer. The next start loads them, and its
match proves the result (executor spec, "Sync", §3b).

The node must be stopped.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		err := debugRepair(cmd.Context(), args[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	},
}

var flagDebugRepair struct {
	Partition string
	Peer      string
}

func init() {
	cmdMain.AddCommand(cmdDebug)
	cmdDebug.AddCommand(cmdDebugRepair)
	cmdDebugRepair.Flags().StringVar(&flagDebugRepair.Partition, "partition", "", "the partition to repair, e.g. BVN1 or Directory")
	cmdDebugRepair.Flags().StringVar(&flagDebugRepair.Peer, "peer", "", "the JSON-RPC v3 endpoint of a running node of the partition")
	_ = cmdDebugRepair.MarkFlagRequired("partition")
	_ = cmdDebugRepair.MarkFlagRequired("peer")
}

func debugRepair(ctx context.Context, dataDir string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	store, logPath, closeStore, err := run.OpenPartitionStore(ctx, dataDir, flagDebugRepair.Partition)
	if err != nil {
		return err
	}
	defer func() { _ = closeStore() }()

	db := database.New(store, nil)
	res, err := join.RepairOffline(ctx, db, protocol.PartitionUrl(flagDebugRepair.Partition), jsonrpc.NewClient(flagDebugRepair.Peer), logPath, slog.Default())
	if err != nil {
		return err
	}
	out, _ := json.MarshalIndent(struct {
		RepairLog string `json:"repairLog"`
		*join.OfflineRepair
	}{logPath, res}, "", "  ")
	fmt.Println(string(out))
	return nil
}

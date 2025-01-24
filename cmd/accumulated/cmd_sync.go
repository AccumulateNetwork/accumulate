// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	cfg "gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"golang.org/x/term"
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
	Use:   "snapshot [tendermint RPC server]",
	Short: "Configures state sync to pull a snapshot",
	Args:  cobra.ExactArgs(1),
	Run:   syncToSnapshot,
}

var cmdRestoreSnapshot = &cobra.Command{
	Use:   "restore-snapshot [file]",
	Short: "Rebuild the accumulate database from a snapshot",
	Args:  cobra.ExactArgs(1),
	Run:   restoreSnapshot,
}

func syncToSnapshot(cmd *cobra.Command, args []string) {
	addr := accumulate.ResolveWellKnownEndpoint(args[0], "")
	netAddr, netPort, err := resolveAddrWithPort(addr)
	checkf(err, "resolve tendermint RPC server")

	client := jsonrpc.NewClient(fmt.Sprintf("http://%s:%d/v3", netAddr, netPort+int(cfg.PortOffsetAccumulateApi)))
	ni, err := client.NodeInfo(cmd.Context(), api.NodeInfoOptions{})
	checkf(err, "get node info from %s", args[0])

	cs, err := client.ConsensusStatus(cmd.Context(), api.ConsensusStatusOptions{
		NodeID: ni.PeerID.String(),
	})
	checkf(err, "get consensus state from %s", args[0])

	c, err := config.Load(flagMain.WorkDir)
	checkf(err, "load configuration")

	tmRPC := fmt.Sprintf("tcp://%s:%d", netAddr, netPort+int(cfg.PortOffsetTendermintRpc))
	err = selectSnapshot(cmd.Context(), c, client, api.ListSnapshotsOptions{
		NodeID:    ni.PeerID.String(),
		Partition: cs.PartitionID,
	}, tmRPC, false)

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

func selectSnapshot(ctx context.Context, config *cfg.Config, client api.SnapshotService, opts api.ListSnapshotsOptions, tmRPC string, skippable bool) error {
	if skippable {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return nil
		}

		i, _, err := (&promptui.Select{
			Label: "Check for snapshots to sync to?",
			Items: []string{
				"Yes",
				"No",
			},
		}).Run()
		if err != nil {
			return fmt.Errorf("select a snapshot: %v", err)
		}
		if i != 0 {
			return nil
		}
	}

	snaps, err := client.ListSnapshots(ctx, opts)
	if err != nil {
		// Don't fail if the node doesn't have a snapshot service
		if errors.Is(err, errors.NotFound) {
			return nil
		}
		return fmt.Errorf("fetch snapshot list: %v", err)
	}

	if len(snaps) == 0 {
		fmt.Println("No snapshots available from this node")
		return nil
	}

	var items []string
	if skippable {
		items = append(items, "Sync from genesis")
	} else {
		items = append(items, "Abort")
	}
	for _, s := range snaps {
		items = append(items, fmt.Sprintf("Block %d (%x)", s.Header.SystemLedger.Index, s.Header.RootHash[:8]))
	}
	i, _, err := (&promptui.Select{Label: "Select a snapshot", Items: items}).Run()
	if err != nil {
		return fmt.Errorf("select a snapshot: %v", err)
	}
	if i == 0 {
		return nil
	}

	snap := snaps[i-1]
	ss := config.StateSync
	ss.Enable = true
	ss.TrustPeriod = 48 * time.Hour
	ss.RPCServers = append(ss.RPCServers, tmRPC)
	ss.TrustHeight = snap.ConsensusInfo.Block.Height
	ss.TrustHash = snap.ConsensusInfo.Block.Header.Hash().String()

	tmClient, err := rpchttp.New(tmRPC, tmRPC+"/websocket")
	if err != nil {
		return fmt.Errorf("create Tendermint client for %s: %v", tmRPC, err)
	}

	tmni, err := tmClient.NetInfo(ctx)
	if err != nil {
		return fmt.Errorf("get network info from node")
	}

	_, netPort, err := resolveAddrWithPort(tmRPC)
	if err != nil {
		return fmt.Errorf("failed to parse port from %q", tmRPC)
	}

	for _, peer := range tmni.Peers {
		addr, err := resolveAddr(peer.RemoteIP)
		if err != nil {
			continue
		}
		ss.RPCServers = append(ss.RPCServers, fmt.Sprintf("tcp://%s:%d", addr, netPort+int(cfg.PortOffsetTendermintRpc)))
	}

	return nil
}

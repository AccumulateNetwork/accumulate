// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/exp/apiutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/fastsync"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/dagbft"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	cmdMain.AddCommand(cmdFastSync)
	cmdFastSync.Flags().StringVar(&flagFastSync.Genesis, "genesis", "", "Genesis snapshot — the trust anchor (required)")
	cmdFastSync.Flags().StringVar(&flagFastSync.Database, "database", "", "Directory for the synced database (required)")
	cmdFastSync.Flags().StringVar(&flagFastSync.Partition, "partition", protocol.Directory, "Partition to sync")
	cmdFastSync.Flags().StringVar(&flagFastSync.RejoinDir, "rejoin-dir", "", "Node work directory to write the consensus rejoin seed into")
	_ = cmdFastSync.MarkFlagRequired("genesis")
	_ = cmdFastSync.MarkFlagRequired("database")
}

var flagFastSync = struct {
	Genesis   string
	Database  string
	Partition string
	RejoinDir string
}{}

var cmdFastSync = &cobra.Command{
	Use:   "fastsync <network>",
	Short: "Deploy node state from the network with cryptographic verification",
	Long: `Fastsync deploys a node's state without replaying history (#4058): it
verifies the signature headers of the major blocks from the genesis snapshot
forward, binds the minor blocks to the present, then restores the current
state and proves it against the verified state tree anchor. The only trust
input is the genesis snapshot.`,
	Args: cobra.ExactArgs(1),
	Run:  runFastSync,
}

func runFastSync(_ *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	network := args[0]

	// The trust anchor
	genesisFile, err := os.Open(flagFastSync.Genesis)
	checkf(err, "open genesis snapshot")
	defer genesisFile.Close()
	partition := config.NetworkUrl{URL: protocol.PartitionUrl(flagFastSync.Partition)}
	genesis, err := fastsync.LoadGenesisGlobals(genesisFile, partition)
	checkf(err, "load trust anchor")
	fmt.Printf("Trust anchor loaded: network version %d, %d validators\n", genesis.Network.Version, len(genesis.Network.Validators))

	// The target database
	db, err := database.OpenBadger(flagFastSync.Database, nil)
	checkf(err, "open database")
	defer db.Close()

	// Connect to the network
	c1 := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(network, "v3"))
	ni, err := c1.NodeInfo(ctx, api.NodeInfoOptions{})
	checkf(err, "query node info")

	node, err := p2p.New(p2p.Options{
		Network:        ni.Network,
		BootstrapPeers: accumulate.BootstrapServers,
	})
	checkf(err, "start p2p node")
	defer func() { _ = node.Close() }()

	router, err := apiutil.InitRouter(apiutil.RouterOptions{
		Context: ctx,
		Node:    node,
		Network: ni.Network,
	})
	checkf(err, "initialize router")
	if ok := <-router.(*routing.RouterInstance).Ready(); !ok {
		fatalf("failed to initialize router")
	}

	client := &message.Client{Transport: &message.RoutedTransport{
		Network: ni.Network,
		Dialer:  node.DialNetwork(),
		Router:  routing.MessageRouter{Router: router},
	}}

	// Sync
	fmt.Printf("Syncing %s from %s\n", partition.URL, network)
	start := time.Now()
	res, err := fastsync.Sync(ctx, fastsync.Options{
		Client:    client.Private().(fastsync.Client),
		Genesis:   genesis,
		Partition: partition,
		Database:  db,
	})
	checkf(err, "sync")

	fmt.Printf("Synced and verified in %v\n", time.Since(start).Round(time.Second))
	fmt.Printf("  Epoch block:       %d (round %d, committee epoch %d)\n", res.Epoch.Block, res.Epoch.Round, res.Epoch.CommitteeEpoch)
	fmt.Printf("  State tree anchor: %x\n", res.Spine.StateTreeAnchor)
	fmt.Printf("  Validators:        %d (network version %d)\n", len(res.Spine.Globals().Network.Validators), res.Spine.Globals().Network.Version)

	// Write the consensus rejoin seed for the node's next startup
	if flagFastSync.RejoinDir != "" {
		if res.Epoch.Round == 0 {
			fmt.Println("WARNING: the server did not report the epoch block's consensus round — no rejoin seed written; the node will start at round zero")
			return
		}
		err = dagbft.WriteRejoinSeed(flagFastSync.RejoinDir, &dagbft.RejoinSeed{
			Partition: flagFastSync.Partition,
			Block:     res.Epoch.Block,
			Round:     res.Epoch.Round,
			Epoch:     res.Epoch.CommitteeEpoch,
		})
		checkf(err, "write rejoin seed")
		fmt.Printf("  Rejoin seed:       %s\n", dagbft.RejoinSeedName(flagFastSync.Partition))
	}
}

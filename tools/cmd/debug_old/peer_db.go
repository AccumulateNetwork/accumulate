// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p/dial"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p/peerdb"
)

var peerDbCmd = &cobra.Command{
	Use:     "peerdb",
	Aliases: []string{"peers", "peer-db"},
}

var peerDbScanCmd = &cobra.Command{
	Use:   "scan [database] [network]",
	Short: "Scan for peers of the given network",
	Args:  cobra.ExactArgs(2),
	Run:   scanPeers,
}

var peerDbDumpCmd = &cobra.Command{
	Use:   "dump [database] [network] [service]",
	Short: "Dump peers for the given network and service",
	Args:  cobra.ExactArgs(3),
	Run:   dumpPeers,
}

func init() {
	cmd.AddCommand(peerDbCmd)
	peerDbCmd.AddCommand(
		peerDbScanCmd,
		peerDbDumpCmd,
	)
}

func scanPeers(_ *cobra.Command, args []string) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	_, sk, err := ed25519.GenerateKey(rand.Reader)
	checkf(err, "generate key")

	node, err := p2p.New(p2p.Options{
		Key:               sk,
		Network:           args[1],
		BootstrapPeers:    bootstrap,
		PeerDatabase:      args[0],
		PeerScanFrequency: -1,
	})
	check(err)
	defer func() { _ = node.Close() }()

	fmt.Println("Waiting for the DHT to initialize...")
	time.Sleep(10 * time.Second)

	tr := node.Tracker().(*dial.PersistentTracker)
	tr.ScanPeers(1 * time.Minute)
}

func dumpPeers(_ *cobra.Command, args []string) {
	b, err := os.ReadFile(args[0])
	check(err)

	db := new(peerdb.DB)
	check(db.Load(bytes.NewReader(b)))

	sa, err := api.ParseServiceAddress(args[2])
	check(err)

	for _, p := range db.Peers.Load() {
		s := p.Network(args[1]).Service(sa)
		if s.Last.Success == nil {
			continue
		}

		fmt.Println(p.ID)
		fmt.Printf("  %v attempt %v\n", sa, s.Last.Attempt)
		fmt.Printf("  %v success %v\n", sa, s.Last.Success)
		for _, a := range p.Addresses.Load() {
			fmt.Printf("  %v attempt %v\n", a.Address, a.Last.Attempt)
			fmt.Printf("  %v success %v\n", a.Address, a.Last.Success)
		}
	}
}

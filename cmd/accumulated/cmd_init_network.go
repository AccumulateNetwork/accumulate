// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/exp/faucet"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	coredb "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gopkg.in/yaml.v3"
)

var cmdInitNetwork = &cobra.Command{
	Use:   "network <network configuration file>",
	Short: "Initialize a network",
	Run:   initNetwork,
	Args:  cobra.MinimumNArgs(1),
}

var cmdInitGenesis = &cobra.Command{
	Use:   "genesis <network configuration file>",
	Short: "Generate genesis files for a network",
	Run:   initGenesis,
	Args:  cobra.MinimumNArgs(1),
}

var cmdInitPrepareGenesis = &cobra.Command{
	Use:   "prepare-genesis <output> <inputs...>",
	Short: "Ingests data from snapshots and produces a single, stripped-down snapshot for genesis",
	Run:   prepareGenesis,
	Args:  cobra.MinimumNArgs(2),
}

func init() {
	cmdInit.AddCommand(cmdInitGenesis, cmdInitPrepareGenesis)
}

func loadNetworkConfiguration(file ...string) *accumulated.NetworkInit {
	network := new(accumulated.NetworkInit)
	for _, file := range file {
		b, err := os.ReadFile(file)
		check(err)

		switch filepath.Ext(file) {
		case ".yml", ".yaml":
			var v any
			check(yaml.Unmarshal(b, &v))
			b, err = json.Marshal(v)
			check(err)
		}

		check(json.Unmarshal(b, &network))
	}

	for _, bvn := range network.Bvns {
		for _, node := range bvn.Nodes {
			if node.PrivValKey == nil {
				node.PrivValKey = generateEd25519Key()
			}
			if node.DnNodeKey == nil {
				node.DnNodeKey = generateEd25519Key()
			}
			if node.BvnNodeKey == nil {
				node.BvnNodeKey = node.DnNodeKey
			}
			if node.BsnNodeKey == nil {
				node.BsnNodeKey = node.DnNodeKey
			}
			if node.ListenAddress == "" {
				node.ListenAddress = "0.0.0.0"
			}
		}
	}
	return network
}

// load network config file
func initNetwork(cmd *cobra.Command, args []string) {
	network := loadNetworkConfiguration(args...)

	if flagInit.Reset {
		networkReset()
	}

	for _, bvn := range network.Bvns {
		for _, node := range bvn.Nodes {
			if node.PrivValKey == nil {
				node.PrivValKey = generateEd25519Key()
			}
			if node.DnNodeKey == nil {
				node.DnNodeKey = generateEd25519Key()
			}
			if node.BvnNodeKey == nil {
				node.BvnNodeKey = generateEd25519Key()
			}
			if node.ListenAddress == "" {
				node.ListenAddress = "0.0.0.0"
			}
		}
	}

	template := new(run.Config)
	template.Logging = new(run.Logging)
	template.P2P = new(run.P2P)
	if network.Template != "" {
		check(template.Load([]byte(network.Template), toml.Unmarshal))
	}

	// Create a dedicated bootstrap node for P2P discovery
	// Use a deterministic key derived from the network ID
	bootstrapSeed := storage.MakeKey(network.Id, "bootstrap")
	bootstrapKey := ed25519.NewKeyFromSeed(bootstrapSeed[:])
	bootstrapAddr := address.FromED25519PrivateKey(bootstrapKey)

	// Compute the bootstrap node's peer ID for multiaddr
	libp2pKey, _ := crypto.UnmarshalEd25519PrivateKey(bootstrapKey)
	bootstrapPeerID, _ := peer.IDFromPrivateKey(libp2pKey)

	// Bootstrap node listens on 127.0.0.1:16658 (before validator ports)
	bootstrapListenAddr := "/ip4/127.0.0.1/tcp/16658"
	bootstrapMultiaddr, _ := multiaddr.NewMultiaddr(bootstrapListenAddr + "/p2p/" + bootstrapPeerID.String())

	// Create bootstrap node config
	{
		dir := filepath.Join(flagMain.WorkDir, "bootstrap")
		check(os.MkdirAll(dir, 0755))

		cfg := template.Copy()
		cfg.Network = network.Id
		cfg.P2P.Key = &run.RawPrivateKey{Address: bootstrapAddr.String()}
		cfg.P2P.DiscoveryMode = run.Ptr(run.DhtMode(dht.ModeAutoServer))
		listenAddr, _ := multiaddr.NewMultiaddr(bootstrapListenAddr)
		cfg.P2P.Listen = []run.Multiaddr{listenAddr}

		// Add RouterService for cross-partition routing
		cfg.Services = append(cfg.Services, &run.RouterService{})

		// Add HttpService for API access
		httpListen, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/16660/http")
		cfg.Services = append(cfg.Services, &run.HttpService{
			HttpListener: run.HttpListener{
				Listen: []run.Multiaddr{httpListen},
			},
			Router: run.ServiceReference[*run.RouterService](""),
		})

		check(cfg.SaveTo(filepath.Join(dir, "accumulate.toml")))
		fmt.Printf("Bootstrap node: %s (peer ID: %s)\n", bootstrapListenAddr, bootstrapPeerID.String())
	}

	genDocs := buildGenesis(network)
	for i, bvn := range network.Bvns {
		for j, node := range bvn.Nodes {
			id := fmt.Sprintf("bvn%d-%d", i+1, j+1)
			dir := filepath.Join(flagMain.WorkDir, id)
			check(os.MkdirAll(dir, 0755))

			cfg := template.Copy()
			cfg.Network = network.Id

			// Add debug logging for DAG consensus modules
			// First rule: DEBUG for consensus modules (must have module= attribute)
			// Second rule: no modules means default level for all other logs
			cfg.Logging = &run.Logging{
				Format: "plain",
				Color:  run.Ptr(true),
				Rules: []*run.LoggingRule{
					{Level: slog.LevelDebug, Modules: []string{"dagbft", "consensus", "primary", "worker", "bullshark", "gossip", "dag", "adapter"}},
					{Level: slog.LevelInfo}, // Default level for logs without "module" attribute
				},
			}

			// Configure the node key
			addr := address.FromED25519PrivateKey(node.DnNodeKey)
			cfg.P2P.Key = &run.RawPrivateKey{Address: addr.String()}

			// Enable DHT server mode for peer discovery
			cfg.P2P.DiscoveryMode = run.Ptr(run.DhtMode(dht.ModeAutoServer))

			// Configure the validator
			cvc := run.AddConfiguration(cfg, new(run.CoreValidatorConfiguration), nil)
			cfg.Configurations = []run.Configuration{cvc}
			cvc.Listen = node.Listen().Scheme("tcp").Directory().TendermintP2P().Multiaddr()
			cvc.BVN = bvn.Id

			// Pin the storage backend in the generated config. The backend of
			// an existing node must never depend on the running binary's
			// default - that could reboot a node into a different (empty)
			// database.
			cvc.StorageType = run.Ptr(run.DefaultStorageType)
			// Use Directory().AccumulateP2P() for all bootstrap peers to match the actual P2P listen port
			// (CoreValidatorConfiguration derives P2P listen from cvc.Listen + portDir + portAccP2P = base + 0 + 2)
			cvc.BvnBootstrapPeers = bvn.Peers(node).Scheme("tcp").Directory().AccumulateP2P().WithKey().Multiaddr()
			cvc.DnBootstrapPeers = network.Peers(node).Scheme("tcp").Directory().AccumulateP2P().WithKey().Multiaddr()

			// Set P2P bootstrap peers - primary is the bootstrap node, then add other peers
			cfg.P2P.BootstrapPeers = []run.Multiaddr{bootstrapMultiaddr}
			cfg.P2P.BootstrapPeers = append(cfg.P2P.BootstrapPeers, cvc.DnBootstrapPeers...)

			// Configure the validator key
			addr = address.FromED25519PrivateKey(node.PrivValKey)
			cvc.ValidatorKey = &run.RawPrivateKey{Address: addr.String()}

			// Workers are shards, so the count is a power of two: the routing
			// key is masked rather than divided, which gives uniform buckets
			// and a cheap stable mapping. 100 was neither a power of two nor a
			// number chosen for a reason, and `% 100` on a hash is not
			// sharding (#4133).
			//
			// Measured (#4164): keyed worker routing is unwired (#4133), so
			// every submission lands on ONE worker regardless of the count —
			// the remaining 63 (×2 nodes per container) were idle batch-timer
			// loops burning wakeups for nothing. A Narwhal worker is meant to
			// be a separate machine contributing its own bandwidth, not 64
			// goroutine-sets sharing one NIC and one disk. Four keeps the
			// power-of-two sharding shape and real parallelism headroom for
			// when #4133 wires the routing key.
			cvc.NumWorkers = run.Ptr(int64(4))

			// Every node serves Prometheus metrics on :26670. Without this no
			// DI node had a /metrics endpoint at all — the CometBFT lineage
			// served one by accident and DAG-BFT removed it without a
			// replacement, so every metrics-fed soak panel (node RSS,
			// goroutines, rcmgr scope usage, heals) read zero for a running
			// network, and the 20260819 resource-exhaustion collapse had no
			// instrument that could have shown it building (#4110, #4115).
			promAddr, _ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/26670/http")
			cfg.Instrumentation = &run.Instrumentation{
				HttpListener: run.HttpListener{Listen: []run.Multiaddr{promAddr}},
			}

			// pprof on EVERY node, not just bvn1-1. On 2026-08-21 all four
			// partitions stopped producing blocks while consensus stayed
			// healthy (#4125); the one artifact that would have said in a
			// single step whether the executor was parked in batch collection
			// is a goroutine dump of a wedged node, and there was no way to
			// take one from 12 of the 13. Bound to 0.0.0.0 so the soak's
			// wedgewatch can reach it with `docker exec <node> curl`.
			pprofAddr, _ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/6060")
			cfg.Instrumentation.PprofListen = pprofAddr

			// Write the genesis documents
			cvc.DnGenesis = "directory-genesis.snap"
			cvc.BvnGenesis = strings.ToLower(bvn.Id) + "-genesis.snap"
			check(os.WriteFile(filepath.Join(dir, cvc.DnGenesis), genDocs[protocol.Directory], 0600))
			check(os.WriteFile(filepath.Join(dir, cvc.BvnGenesis), genDocs[bvn.Id], 0600))

			// Write the node configuration
			check(cfg.SaveTo(filepath.Join(dir, "accumulate.toml")))
		}
	}
}

func initGenesis(cmd *cobra.Command, args []string) {
	network := loadNetworkConfiguration(args...)

	// Generate genesis docs
	genDocs := buildGenesis(network)

	// Write documents, as binary and as JSON
	check(os.MkdirAll(flagMain.WorkDir, 0755))
	for part, snap := range genDocs {
		check(os.WriteFile(filepath.Join(flagMain.WorkDir, part+".snap"), snap, 0600))
		doc, err := genesis.ConvertSnapshotToJson(snap)
		check(err)
		snap, err = json.MarshalIndent(doc, "", "  ")
		check(err)
		check(os.WriteFile(filepath.Join(flagMain.WorkDir, part+".json"), snap, 0600))
	}
}

func prepareGenesis(cmd *cobra.Command, args []string) {
	// Timer for updating progress
	tick := time.NewTicker(time.Second / 2)
	defer tick.Stop()

	db := coredb.OpenInMemory(nil)
	for _, path := range args[1:] {
		fmt.Println("Processing", path)
		file, err := os.Open(path)
		check(err)
		defer file.Close()
		_, err = genesis.Extract(db, file, func(u *url.URL) bool {
			select {
			case <-tick.C:
				h := database.NewKey("Account", u).Hash()
				fmt.Printf("\033[A\r\033[KProcessing [%x] %v\n", h[:4], u)
			default:
				return true
			}

			// Retain everything
			return true
		})
		check(err)
	}

	file, err := os.Create(args[0])
	check(err)
	defer file.Close()

	// Collect
	var metrics coredb.CollectMetrics
	fmt.Println("Collecting into", args[0])
	_, err = db.Collect(file, nil, &coredb.CollectOptions{
		Metrics: &metrics,
		Predicate: func(r database.Record) (bool, error) {
			select {
			case <-tick.C:
			default:
				return true, nil
			}

			// Print progress
			switch r.Key().Get(0) {
			case "Account":
				k := r.Key().SliceJ(2)
				h := k.Hash()
				fmt.Printf("\033[A\r\033[KCollecting [%x] (%d) %v\n", h[:4], metrics.Messages.Count, k.Get(1))

			case "Message", "Transaction":
				fmt.Printf("\033[A\r\033[KCollecting (%d/%d) %x\n", metrics.Messages.Collecting, metrics.Messages.Count, r.Key().Get(1).([32]byte))
			}

			// Retain everything
			return true, nil
		},
	})
	check(err)
}

func buildGenesis(network *accumulated.NetworkInit) map[string][]byte {
	var factomAddresses func() (io.Reader, error)
	var snapshots []func(*core.GlobalValues) (ioutil2.SectionReader, error)
	if flagInit.FactomAddresses != "" {
		factomAddresses = func() (io.Reader, error) { return os.Open(flagInit.FactomAddresses) }
	}
	for _, filename := range flagInit.Snapshots {
		filename := filename // See docs/developer/rangevarref.md
		snapshots = append(snapshots, func(*core.GlobalValues) (ioutil2.SectionReader, error) { return os.Open(filename) })
	}
	if flagInit.FaucetSeed != "" {
		b := createFaucet(strings.Split(flagInit.FaucetSeed, " "))
		snapshots = append(snapshots, func(*core.GlobalValues) (ioutil2.SectionReader, error) {
			return ioutil2.NewBuffer(b), nil
		})
	}

	genDocs, err := accumulated.BuildGenesisDocs(network, network.Globals, time.Now(), newLogger(), factomAddresses, snapshots)
	checkf(err, "build genesis documents")
	return genDocs
}

func createFaucet(seedStrs []string) []byte {
	var seed storage.Key
	for _, s := range seedStrs {
		seed = seed.Append(s)
	}
	sk := ed25519.NewKeyFromSeed(seed[:])

	u, err := protocol.LiteTokenAddress(sk[32:], "ACME", protocol.SignatureTypeED25519)
	check(err)
	fmt.Printf("Faucet: %v\n", u)
	b, err := faucet.CreateLite(u)
	check(err)
	return b
}

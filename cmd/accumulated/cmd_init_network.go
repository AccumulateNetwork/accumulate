// Copyright 2025 The Accumulate Authors
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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	cmtjson "github.com/cometbft/cometbft/libs/json"
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
	"gitlab.com/accumulatenetwork/accumulate/test/testing"
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
				node.PrivValKey = tmed25519.GenPrivKey()
			}
			if node.DnNodeKey == nil {
				node.DnNodeKey = tmed25519.GenPrivKey()
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
				node.PrivValKey = tmed25519.GenPrivKey()
			}
			if node.DnNodeKey == nil {
				node.DnNodeKey = tmed25519.GenPrivKey()
			}
			if node.BvnNodeKey == nil {
				node.BvnNodeKey = tmed25519.GenPrivKey()
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

	genDocs := buildGenesis(network)
	for i, bvn := range network.Bvns {
		for j, node := range bvn.Nodes {
			id := fmt.Sprintf("bvn%d-%d", i+1, j+1)
			dir := filepath.Join(flagMain.WorkDir, id)
			check(os.MkdirAll(dir, 0755))

			cfg := template.Copy()
			cfg.Network = network.Id

			// TODO: log levels

			// Configure the node key
			addr := address.FromED25519PrivateKey(node.DnNodeKey)
			cfg.P2P.Key = &run.RawPrivateKey{Address: addr.String()}

			// Configure the validator
			cvc := run.AddConfiguration(cfg, new(run.CoreValidatorConfiguration), nil)
			cfg.Configurations = []run.Configuration{cvc}
			cvc.Listen = node.Listen().Scheme("tcp").Directory().TendermintP2P().Multiaddr()
			cvc.BVN = bvn.Id
			cvc.BvnBootstrapPeers = bvn.Peers(node).Scheme("tcp").BlockValidator().TendermintP2P().WithKey().Multiaddr()
			cvc.DnBootstrapPeers = network.Peers(node).Scheme("tcp").Directory().TendermintP2P().WithKey().Multiaddr()

			// Configure the validator key
			addr = address.FromED25519PrivateKey(node.PrivValKey)
			cvc.ValidatorKey = &run.RawPrivateKey{Address: addr.String()}

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
		snap, err = cmtjson.MarshalIndent(doc, "", "  ")
		check(err)
		check(os.WriteFile(filepath.Join(flagMain.WorkDir, part+".json"), snap, 0600))
	}
}

func prepareGenesis(cmd *cobra.Command, args []string) {
	// Timer for updating progress
	tick := time.NewTicker(time.Second / 2)
	defer tick.Stop()

	db := coredb.OpenInMemory(nil)
	db.SetObserver(testing.NullObserver{})
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

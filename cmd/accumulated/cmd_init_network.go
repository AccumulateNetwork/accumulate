// Copyright 2023 The Accumulate Authors
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
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	cfg "gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	etcd "go.etcd.io/etcd/client/v3"
)

var cmdInitNetwork = &cobra.Command{
	Use:   "network <network configuration file>",
	Short: "Initialize a network",
	Run:   initNetwork,
	Args:  cobra.ExactArgs(1),
}

func loadNetworkConfiguration(file string) (ret *accumulated.NetworkInit, err error) {
	jsonFile, err := os.Open(file)
	defer func() { _ = jsonFile.Close() }()
	// if we os.Open returns an error then handle it
	if err != nil {
		return ret, err
	}
	data, _ := io.ReadAll(jsonFile)
	err = json.Unmarshal(data, &ret)
	return ret, err
}

// load network config file
func initNetwork(cmd *cobra.Command, args []string) {
	networkConfigFile := args[0]
	network, err := loadNetworkConfiguration(networkConfigFile)
	check(err)

	verifyInitFlags(cmd, len(network.Bvns))

	if flagInit.Reset {
		networkReset()
	}

	for _, bvn := range network.Bvns {
		for _, node := range bvn.Nodes {
			// TODO Check for existing keys?
			node.PrivValKey = tmed25519.GenPrivKey()
			node.DnNodeKey = tmed25519.GenPrivKey()
			node.BvnNodeKey = tmed25519.GenPrivKey()

			if node.ListenAddress == "" {
				node.ListenAddress = "0.0.0.0"
			}
		}
	}

	initNetworkLocalFS(cmd, network)
}

func verifyInitFlags(cmd *cobra.Command, count int) {
	if flagInitDevnet.Compose {
		flagInitDevnet.Docker = true
	}

	if flagInitDevnet.Docker && cmd.Flag("ip").Changed {
		fatalf("--ip and --docker are mutually exclusive")
	}

	if count == 0 {
		fatalf("Must have at least one node")
	}

	switch len(flagInitDevnet.IPs) {
	case 1:
		// Generate a sequence from the base IP
	case count * flagInitDevnet.NumBvns:
		// One IP per node
	default:
		fatalf("not enough IPs - you must specify one base IP or one IP for each node")
	}
}

func initNetworkLocalFS(cmd *cobra.Command, netInit *accumulated.NetworkInit) {
	if flagInit.LogLevels != "" {
		_, _, err := logging.ParseLogLevel(flagInit.LogLevels, io.Discard)
		checkf(err, "--log-level")
	}

	check(os.MkdirAll(flagMain.WorkDir, 0755))

	netFile, err := os.Create(filepath.Join(flagMain.WorkDir, "network.json"))
	check(err)
	enc := json.NewEncoder(netFile)
	enc.SetIndent("", "    ")
	check(enc.Encode(netInit))
	check(netFile.Close())

	var factomAddresses func() (io.Reader, error)
	var snapshots []func() (ioutil2.SectionReader, error)
	if flagInit.FactomAddresses != "" {
		factomAddresses = func() (io.Reader, error) { return os.Open(flagInit.FactomAddresses) }
	}
	for _, filename := range flagInit.Snapshots {
		filename := filename // See docs/developer/rangevarref.md
		snapshots = append(snapshots, func() (ioutil2.SectionReader, error) { return os.Open(filename) })
	}
	if flagInit.FaucetSeed != "" {
		b := createFaucet(strings.Split(flagInit.FaucetSeed, " "))
		snapshots = append(snapshots, func() (ioutil2.SectionReader, error) {
			return ioutil2.NewBuffer(b), nil
		})
	}

	values := new(core.GlobalValues)
	if flagInitDevnet.Globals != "" {
		checkf(yaml.Unmarshal([]byte(flagInitDevnet.Globals), values), "--globals")
	}

	genDocs, err := accumulated.BuildGenesisDocs(netInit, values, time.Now(), newLogger(), factomAddresses, snapshots)
	checkf(err, "build genesis documents")

	configs := accumulated.BuildNodesConfig(netInit, nil)
	var count int
	dnGenDoc := genDocs[protocol.Directory]
	for i, bvn := range netInit.Bvns {
		bvnGenDoc := genDocs[bvn.Id]
		for j, node := range bvn.Nodes {
			count++
			configs[i][j][0].SetRoot(filepath.Join(flagMain.WorkDir, fmt.Sprintf("node-%d", count), "dnn"))
			configs[i][j][1].SetRoot(filepath.Join(flagMain.WorkDir, fmt.Sprintf("node-%d", count), "bvnn"))

			for _, config := range configs[i][j] {
				if flagInit.LogLevels != "" {
					config.LogLevel = flagInit.LogLevels
				}

				if flagInit.NoEmptyBlocks {
					config.Consensus.CreateEmptyBlocks = false
				}

				if len(flagInit.Etcd) > 0 {
					config.Accumulate.Storage.Type = cfg.EtcdStorage
					config.Accumulate.Storage.Etcd = new(etcd.Config)
					config.Accumulate.Storage.Etcd.Endpoints = flagInit.Etcd
					config.Accumulate.Storage.Etcd.DialTimeout = 5 * time.Second
				}
			}
			configs[i][j][0].Config.PrivValidatorKey = "../priv_validator_key.json"
			err = accumulated.WriteNodeFiles(configs[i][j][0], node.PrivValKey, node.DnNodeKey, dnGenDoc)
			checkf(err, "write DNN files")
			configs[i][j][1].Config.PrivValidatorKey = "../priv_validator_key.json"
			err = accumulated.WriteNodeFiles(configs[i][j][1], node.PrivValKey, node.BvnNodeKey, bvnGenDoc)
			checkf(err, "write BVNN files")

		}
	}
}

func createFaucet(seedStrs []string) []byte {
	var seed storage.Key
	for _, s := range seedStrs {
		seed = seed.Append(s)
	}
	sk := ed25519.NewKeyFromSeed(seed[:])

	store := memory.New(nil)
	batch := store.Begin(true)
	defer batch.Discard()
	bpt := pmt.NewBPTManager(batch, storage.MakeKey("BPT"))

	var err error
	lta := new(protocol.LiteTokenAccount)
	lta.Url, err = protocol.LiteTokenAddress(sk[32:], "ACME", protocol.SignatureTypeED25519)
	check(err)
	lta.TokenUrl = protocol.AcmeUrl()
	lta.Balance = *big.NewInt(200_000_000 * protocol.AcmePrecision)
	fmt.Printf("Faucet: %v\n", lta.Url)

	hasher := make(hash.Hasher, 4)
	lookup := map[[32]byte]*snapshot.Account{}

	hasher = hasher[:0]
	b, _ := lta.MarshalBinary()
	hasher.AddBytes(b)
	hasher.AddHash(new([32]byte))
	hasher.AddHash(new([32]byte))
	hasher.AddHash(new([32]byte))
	bpt.InsertKV(lta.Url.AccountID32(), *(*[32]byte)(hasher.MerkleHash()))
	lookup[lta.Url.AccountID32()] = &snapshot.Account{Main: lta}

	lid := new(protocol.LiteIdentity)
	lid.Url = lta.Url.RootIdentity()
	lid.CreditBalance = math.MaxUint64
	a := new(snapshot.Account)
	a.Url = lid.Url
	a.Main = lid
	a.Directory = []*url.URL{lta.Url}
	hasher = hasher[:0]
	b, _ = lid.MarshalBinary()
	hasher.AddBytes(b)
	hasher.AddValue(hashSecondaryState(a))
	hasher.AddHash(new([32]byte))
	hasher.AddHash(new([32]byte))
	bpt.InsertKV(lid.Url.AccountID32(), *(*[32]byte)(hasher.MerkleHash()))
	lookup[lid.Url.AccountID32()] = a

	check(bpt.Bpt.Update())

	buf := new(ioutil2.Buffer)
	w, err := snapshot.Create(buf, new(snapshot.Header))
	checkf(err, "initialize snapshot")
	sw, err := w.Open(snapshot.SectionTypeAccounts)
	checkf(err, "open accounts snapshot")
	check(bpt.Bpt.SaveSnapshot(sw, func(key storage.Key, _ [32]byte) ([]byte, error) {
		b, err := lookup[key].MarshalBinary()
		if err != nil {
			return nil, errors.EncodingError.WithFormat("marshal account: %w", err)
		}
		return b, nil
	}))
	check(sw.Close())
	return buf.Bytes()
}

func hashSecondaryState(a *snapshot.Account) hash.Hasher {
	var hasher hash.Hasher
	for _, u := range a.Directory {
		hasher.AddUrl(u)
	}
	// Hash the hash to allow for future expansion
	dirHash := hasher.MerkleHash()
	return hash.Hasher{dirHash}
}

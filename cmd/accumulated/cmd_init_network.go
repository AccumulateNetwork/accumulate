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
	"os"
	"path/filepath"
	"strings"
	"time"

	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/exp/faucet"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmdInitNetwork = &cobra.Command{
	Use:   "network <network configuration file>",
	Short: "Initialize a network",
	Run:   initNetwork,
	Args:  cobra.ExactArgs(1),
}

var cmdInitGenesis = &cobra.Command{
	Use:   "genesis <network configuration file>",
	Short: "Generate genesis files for a network",
	Run:   initGenesis,
	Args:  cobra.ExactArgs(1),
}

func init() {
	cmdInit.AddCommand(cmdInitGenesis)
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
	if err != nil {
		return nil, err
	}

	for _, bvn := range ret.Bvns {
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

	initNetworkLocalFS(cmd, network)
}

func initGenesis(cmd *cobra.Command, args []string) {
	networkConfigFile := args[0]
	network, err := loadNetworkConfiguration(networkConfigFile)
	check(err)

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

	configs := accumulated.BuildNodesConfig(netInit, nil)
	for _, configs := range configs {
		for _, configs := range configs {
			for _, config := range configs {
				// Use binary genesis files
				config.Genesis = "config/genesis.snap"

				if flagInit.LogLevels != "" {
					config.LogLevel = flagInit.LogLevels
				}

				if flagInit.NoEmptyBlocks {
					config.Consensus.CreateEmptyBlocks = false
				}
			}
		}
	}

	var count int
	genDocs := buildGenesis(netInit)
	dnGenDoc := genDocs[protocol.Directory]
	for i, bvn := range netInit.Bvns {
		bvnGenDoc := genDocs[bvn.Id]
		for j, node := range bvn.Nodes {
			count++
			configs[i][j][0].SetRoot(filepath.Join(flagMain.WorkDir, fmt.Sprintf("node-%d", count), "dnn"))
			configs[i][j][1].SetRoot(filepath.Join(flagMain.WorkDir, fmt.Sprintf("node-%d", count), "bvnn"))

			configs[i][j][0].Config.PrivValidatorKey = "../priv_validator_key.json"
			err = accumulated.WriteNodeFiles(configs[i][j][0], node.PrivValKey, node.DnNodeKey, dnGenDoc)
			checkf(err, "write DNN files")
			configs[i][j][1].Config.PrivValidatorKey = "../priv_validator_key.json"
			err = accumulated.WriteNodeFiles(configs[i][j][1], node.PrivValKey, node.BvnNodeKey, bvnGenDoc)
			checkf(err, "write BVNN files")
		}
	}

	if netInit.Bsn != nil {
		bsnGenDoc := genDocs[netInit.Bsn.Id]
		i := len(netInit.Bvns)
		for j, node := range netInit.Bsn.Nodes {
			configs[i][j][0].SetRoot(filepath.Join(flagMain.WorkDir, fmt.Sprintf("bsn-%d", j+1), "bsnn"))

			configs[i][j][0].Config.PrivValidatorKey = "../priv_validator_key.json"
			err = accumulated.WriteNodeFiles(configs[i][j][0], node.PrivValKey, node.BsnNodeKey, bsnGenDoc)
			checkf(err, "write BSNN files")
		}
	}

	if netInit.Bootstrap != nil {
		i := len(netInit.Bvns)
		if netInit.Bsn != nil {
			i++
		}
		configs[i][0][0].SetRoot(filepath.Join(flagMain.WorkDir, "bootstrap"))

		err = accumulated.WriteNodeFiles(configs[i][0][0], nil, netInit.Bootstrap.PrivValKey, nil)
		checkf(err, "write bootstrap files")
	}
}

func buildGenesis(network *accumulated.NetworkInit) map[string][]byte {
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

	genDocs, err := accumulated.BuildGenesisDocs(network, values, time.Now(), newLogger(), factomAddresses, snapshots)
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

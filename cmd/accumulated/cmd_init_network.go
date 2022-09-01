package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"
	"github.com/tendermint/tendermint/crypto/ed25519"
	cfg "gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
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

//load network config file
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
			node.PrivValKey = ed25519.GenPrivKey()
			node.NodeKey = ed25519.GenPrivKey()

			if node.ListenIP == "" {
				node.ListenIP = "0.0.0.0"
			}
		}
	}

	initNetworkLocalFS(network)
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

func initNetworkLocalFS(netInit *accumulated.NetworkInit) {
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

	values := new(core.GlobalValues)
	if flagInitDevnet.Globals != "" {
		checkf(yaml.Unmarshal([]byte(flagInitDevnet.Globals), values), "--globals")
	}

	genDocs, err := accumulated.BuildGenesisDocs(netInit, values, time.Now(), newLogger(), nil)
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
			configs[i][j][0].Config.PrivValidator.Key = "../priv_validator_key.json"
			err = accumulated.WriteNodeFiles(configs[i][j][0], node.PrivValKey, node.NodeKey, dnGenDoc)
			checkf(err, "write DNN files")
			configs[i][j][1].Config.PrivValidator.Key = "../priv_validator_key.json"
			err = accumulated.WriteNodeFiles(configs[i][j][1], node.PrivValKey, node.NodeKey, bvnGenDoc)
			checkf(err, "write BVNN files")

		}
	}
}

// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/tendermint/tendermint/crypto/ed25519"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	tmjson "github.com/tendermint/tendermint/libs/json"
	"github.com/tendermint/tendermint/libs/log"
	tmos "github.com/tendermint/tendermint/libs/os"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	tmtypes "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const nodeDirPerm = 0755

type MakeConfigFunc func(networkName string, net protocol.PartitionType, node config.NodeType, netId string) *config.Config

func BuildNodesConfig(network *NetworkInit, mkcfg MakeConfigFunc) [][][2]*config.Config {
	var allConfigs [][][2]*config.Config

	if mkcfg == nil {
		mkcfg = config.Default
	}

	netConfig := config.Network{Id: network.Id, Partitions: make([]config.Partition, 1)}
	dnConfig := config.Partition{
		Id:       protocol.Directory,
		Type:     protocol.PartitionTypeDirectory,
		BasePort: int64(network.Bvns[0].Nodes[0].BasePort), // TODO This is not great
	}

	// If the node addresses are loopback or private IPs, disable strict address book
	ip := net.ParseIP(network.Bvns[0].Nodes[0].Peer().String())
	strict := ip == nil || !(ip.IsLoopback() || ip.IsPrivate())

	var i int
	for _, bvn := range network.Bvns {
		var bvnConfigs [][2]*config.Config
		bvnConfig := config.Partition{
			Id:       bvn.Id,
			Type:     protocol.PartitionTypeBlockValidator,
			BasePort: int64(bvn.Nodes[0].BasePort) + int64(config.PortOffsetBlockValidator), // TODO This is not great
		}
		for j, node := range bvn.Nodes {
			i++
			dnn := mkcfg(network.Id, protocol.PartitionTypeDirectory, node.DnnType, protocol.Directory)
			dnn.Moniker = fmt.Sprintf("Directory.%d", i)
			ConfigureNodePorts(node, dnn, protocol.PartitionTypeDirectory)
			dnConfig.Nodes = append(dnConfig.Nodes, config.Node{
				Address: node.Advertize().Scheme("http").TendermintP2P().Directory().String(),
				Type:    node.DnnType,
			})

			bvnn := mkcfg(network.Id, protocol.PartitionTypeBlockValidator, node.BvnnType, bvn.Id)
			bvnn.Moniker = fmt.Sprintf("%s.%d", bvn.Id, j+1)
			ConfigureNodePorts(node, bvnn, protocol.PartitionTypeBlockValidator)
			bvnConfig.Nodes = append(bvnConfig.Nodes, config.Node{
				Address: node.Advertize().Scheme("http").TendermintP2P().BlockValidator().String(),
				Type:    node.BvnnType,
			})

			if dnn.P2P.ExternalAddress == "" {
				dnn.P2P.ExternalAddress = node.Peer().TendermintP2P().Directory().String()
			}
			if bvnn.P2P.ExternalAddress == "" {
				bvnn.P2P.ExternalAddress = node.Peer().TendermintP2P().BlockValidator().String()
			}

			// No duplicate IPs
			dnn.P2P.AllowDuplicateIP = false
			bvnn.P2P.AllowDuplicateIP = false

			// Initial peers (should be bootstrap peers but that setting isn't
			// present in 0.37)
			dnn.P2P.PersistentPeers = strings.Join(network.Peers(node).Directory().TendermintP2P().WithKey().String(), ",")
			bvnn.P2P.PersistentPeers = strings.Join(bvn.Peers(node).BlockValidator().TendermintP2P().WithKey().String(), ",")

			// Set whether unroutable addresses are allowed
			dnn.P2P.AddrBookStrict = strict
			bvnn.P2P.AddrBookStrict = strict

			p2pPeers := network.Peers(node).AccumulateP2P().WithKey().
				Do(AddressBuilder.Directory, AddressBuilder.BlockValidator).
				Do(func(b AddressBuilder) AddressBuilder { return b.Scheme("tcp") }, func(b AddressBuilder) AddressBuilder { return b.Scheme("udp") }).
				Multiaddr()
			dnn.Accumulate.P2P.BootstrapPeers = p2pPeers
			bvnn.Accumulate.P2P.BootstrapPeers = p2pPeers

			bvnConfigs = append(bvnConfigs, [2]*config.Config{dnn, bvnn})
		}
		allConfigs = append(allConfigs, bvnConfigs)
		netConfig.Partitions = append(netConfig.Partitions, bvnConfig)
	}
	netConfig.Partitions[0] = dnConfig

	for _, configs := range allConfigs {
		for _, configs := range configs {
			for _, config := range configs {
				config.Accumulate.Network = netConfig
			}
		}
	}

	return allConfigs
}

func ConfigureNodePorts(node *NodeInit, cfg *config.Config, part protocol.PartitionType) {
	cfg.P2P.ListenAddress = node.Listen().Scheme("tcp").PartitionType(part).TendermintP2P().String()
	cfg.RPC.ListenAddress = node.Listen().Scheme("tcp").PartitionType(part).TendermintRPC().String()

	cfg.Instrumentation.PrometheusListenAddr = node.Listen().PartitionType(part).Prometheus().String()
	if cfg.Accumulate.LocalAddress == "" {
		cfg.Accumulate.LocalAddress = node.Advertize().PartitionType(part).TendermintP2P().String()
	}
	cfg.Accumulate.P2P.Listen = []multiaddr.Multiaddr{
		node.Listen().Scheme("tcp").PartitionType(part).AccumulateP2P().Multiaddr(),
		node.Listen().Scheme("udp").PartitionType(part).AccumulateP2P().Multiaddr(),
	}
	cfg.Accumulate.API.ListenAddress = node.Listen().Scheme("http").PartitionType(part).AccumulateAPI().String()
}

func BuildGenesisDocs(network *NetworkInit, globals *core.GlobalValues, time time.Time, logger log.Logger, factomAddresses func() (io.Reader, error), snapshots []func() (ioutil2.SectionReader, error)) (map[string]*tmtypes.GenesisDoc, error) {
	docs := map[string]*tmtypes.GenesisDoc{}
	var operators [][]byte
	netinfo := new(protocol.NetworkDefinition)
	netinfo.NetworkName = network.Id
	netinfo.AddPartition(protocol.Directory, protocol.PartitionTypeDirectory)

	var dnTmValidators []tmtypes.GenesisValidator

	var i int
	for _, bvn := range network.Bvns {
		var bvnTmValidators []tmtypes.GenesisValidator

		for j, node := range bvn.Nodes {
			i++
			key := tmed25519.PrivKey(node.PrivValKey)
			operators = append(operators, key.PubKey().Bytes())

			netinfo.AddValidator(key.PubKey().Bytes(), protocol.Directory, node.DnnType == config.Validator)
			netinfo.AddValidator(key.PubKey().Bytes(), bvn.Id, node.BvnnType == config.Validator)

			if node.DnnType == config.Validator {
				dnTmValidators = append(dnTmValidators, tmtypes.GenesisValidator{
					Name:    fmt.Sprintf("Directory.%d", i),
					Address: key.PubKey().Address(),
					PubKey:  key.PubKey(),
					Power:   1,
				})
			}

			if node.BvnnType == config.Validator {
				bvnTmValidators = append(bvnTmValidators, tmtypes.GenesisValidator{
					Name:    fmt.Sprintf("%s.%d", bvn.Id, j+1),
					Address: key.PubKey().Address(),
					PubKey:  key.PubKey(),
					Power:   1,
				})
			}
		}

		netinfo.AddPartition(bvn.Id, protocol.PartitionTypeBlockValidator)
		docs[bvn.Id] = &tmtypes.GenesisDoc{
			ChainID:         bvn.Id,
			GenesisTime:     time,
			InitialHeight:   protocol.GenesisBlock + 1,
			Validators:      bvnTmValidators,
			ConsensusParams: tmtypes.DefaultConsensusParams(),
		}
	}

	docs[protocol.Directory] = &tmtypes.GenesisDoc{
		ChainID:         protocol.Directory,
		GenesisTime:     time,
		InitialHeight:   protocol.GenesisBlock + 1,
		Validators:      dnTmValidators,
		ConsensusParams: tmtypes.DefaultConsensusParams(),
	}

	globals.Network = netinfo

	for id := range docs {
		netType := protocol.PartitionTypeBlockValidator
		if strings.EqualFold(id, protocol.Directory) {
			netType = protocol.PartitionTypeDirectory
		}
		snapshot := new(ioutil2.Buffer)
		root, err := genesis.Init(snapshot, genesis.InitOpts{
			PartitionId:     id,
			NetworkType:     netType,
			GenesisTime:     time,
			Logger:          logger.With("partition", id),
			GenesisGlobals:  globals,
			OperatorKeys:    operators,
			FactomAddresses: factomAddresses,
			Snapshots:       snapshots,
		})
		if err != nil {
			return nil, err
		}

		docs[id].AppHash = root
		docs[id].AppState, err = json.Marshal(snapshot.Bytes())
		if err != nil {
			return nil, err
		}
	}

	return docs, nil
}

func WriteNodeFiles(cfg *config.Config, privValKey, nodeKey []byte, genDoc *tmtypes.GenesisDoc) (err error) {
	defer func() {
		if err != nil {
			_ = os.RemoveAll(cfg.RootDir)
		}
	}()

	// Create directories
	err = os.MkdirAll(filepath.Join(cfg.RootDir, "config"), nodeDirPerm)
	if err != nil {
		return fmt.Errorf("failed to create config dir: %v", err)
	}

	err = os.MkdirAll(filepath.Join(cfg.RootDir, "data"), nodeDirPerm)
	if err != nil {
		return fmt.Errorf("failed to create data dir: %v", err)
	}

	// Write files
	err = config.Store(cfg)
	if err != nil {
		return fmt.Errorf("failed to write config files: %w", err)
	}

	err = loadOrCreatePrivVal(cfg, privValKey)
	if err != nil {
		return fmt.Errorf("failed to write private validator: %w", err)
	}

	err = loadOrCreateNodeKey(cfg, nodeKey)
	if err != nil {
		return fmt.Errorf("failed to write node key: %w", err)
	}

	err = genDoc.SaveAs(cfg.GenesisFile())
	if err != nil {
		return fmt.Errorf("failed to write genesis file: %w", err)
	}

	return nil
}

func loadOrCreatePrivVal(cfg *config.Config, key []byte) error {
	keyFile := cfg.PrivValidatorKeyFile()
	stateFile := cfg.PrivValidatorStateFile()
	if !tmos.FileExists(keyFile) {
		pv := privval.NewFilePV(tmed25519.PrivKey(key), keyFile, stateFile)
		pv.Save()
		return nil
	}
	var pv *privval.FilePV
	var err error
	if !tmos.FileExists(stateFile) {
		// When initializing the other node, the key file has already been created
		pv = privval.NewFilePV(tmed25519.PrivKey(key), keyFile, stateFile)
		pv.LastSignState.Save()
		// Don't return here - we still need to check that the key on disk matches what we expect
	} else { // if file exists then we need to load it
		pv, err = config.LoadFilePV(keyFile, stateFile)
		if err != nil {
			return err
		}
	}

	if !bytes.Equal(pv.Key.PrivKey.Bytes(), key) {
		return fmt.Errorf("existing private key does not match try using --reset flag")
	}

	return nil
}

func loadOrCreateNodeKey(config *config.Config, key []byte) error {
	keyFile := config.NodeKeyFile()
	if !tmos.FileExists(keyFile) {
		nodeKey := p2p.NodeKey{
			PrivKey: ed25519.PrivKey(key),
		}
		return nodeKey.SaveAs(keyFile)
	}

	nodeKey, err := p2p.LoadNodeKey(keyFile)
	if err != nil {
		return err
	}

	if !bytes.Equal(nodeKey.PrivKey.Bytes(), key) {
		return fmt.Errorf("existing private key does not match try using --reset flag")
	}

	return nil
}

func LoadOrGenerateTmPrivKey(privFileName string) (tmed25519.PrivKey, error) {
	//attempt to load the priv validator key, create otherwise.
	b, err := os.ReadFile(privFileName)
	var privValKey tmed25519.PrivKey
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			//do not overwrite a private validator key.
			return tmed25519.GenPrivKey(), nil
		}
		return nil, err
	}
	var pvkey privval.FilePVKey
	err = tmjson.Unmarshal(b, &pvkey)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal existing private validator from %s: %v try using --reset flag", privFileName, err)
	} else {
		privValKey = pvkey.PrivKey.(tmed25519.PrivKey)
	}

	return privValKey, nil
}

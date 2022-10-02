// Copyright 2022 The Accumulate Authors
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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tendermint/tendermint/crypto/ed25519"
	tmjson "github.com/tendermint/tendermint/libs/json"
	"github.com/tendermint/tendermint/libs/log"
	tmos "github.com/tendermint/tendermint/libs/os"
	"github.com/tendermint/tendermint/privval"
	tmtypes "github.com/tendermint/tendermint/types"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/genesis"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const nodeDirPerm = 0755

type AddressType int

const (
	ListenAddress AddressType = iota
	AdvertizeAddress
	PeerAddress
)

func (n *NodeInit) Port(offset ...config.PortOffset) int {
	port := int(n.BasePort)
	for _, o := range offset {
		port += int(o)
	}
	return port
}

func (n *NodeInit) Address(typ AddressType, scheme string, offset ...config.PortOffset) string {
	addr := n.AdvertizeAddress
	switch {
	case typ == ListenAddress && n.ListenAddress != "":
		addr = n.ListenAddress
	case typ == PeerAddress && n.PeerAddress != "":
		addr = n.PeerAddress
	}

	if scheme == "" {
		return fmt.Sprintf("%s:%d", addr, n.Port(offset...))
	}
	return fmt.Sprintf("%s://%s:%d", scheme, addr, n.Port(offset...))
}

func (b *BvnInit) Peers(node *NodeInit, offset ...config.PortOffset) []string {
	var peers []string
	for _, n := range b.Nodes {
		if n != node {
			nodeId := tmtypes.NodeIDFromPubKey(ed25519.PubKey(n.NodeKey[32:]))
			addr := nodeId.AddressString(n.Address(PeerAddress, "", offset...))
			peers = append(peers, addr)
		}
	}
	return peers
}

func (n *NetworkInit) Peers(node *NodeInit, offset ...config.PortOffset) []string {
	var peers []string
	for _, b := range n.Bvns {
		peers = append(peers, b.Peers(node, offset...)...)
	}
	return peers
}

type MakeConfigFunc func(networkName string, net config.NetworkType, node config.NodeType, netId string) *config.Config

func BuildNodesConfig(network *NetworkInit, mkcfg MakeConfigFunc) [][][2]*config.Config {
	var allConfigs [][][2]*config.Config

	if mkcfg == nil {
		mkcfg = config.Default
	}

	netConfig := config.Network{Id: network.Id, Partitions: make([]config.Partition, 1)}
	dnConfig := config.Partition{
		Id:       protocol.Directory,
		Type:     config.Directory,
		BasePort: int64(network.Bvns[0].Nodes[0].BasePort), // TODO This is not great
	}

	var i int
	for _, bvn := range network.Bvns {
		var bvnConfigs [][2]*config.Config
		bvnConfig := config.Partition{
			Id:       bvn.Id,
			Type:     config.BlockValidator,
			BasePort: int64(bvn.Nodes[0].BasePort) + int64(config.PortOffsetBlockValidator), // TODO This is not great
		}
		for j, node := range bvn.Nodes {
			i++
			dnn := mkcfg(network.Id, config.Directory, node.DnnType, protocol.Directory)
			dnn.Moniker = fmt.Sprintf("Directory.%d", i)
			ConfigureNodePorts(node, dnn, config.PortOffsetDirectory)
			dnConfig.Nodes = append(dnConfig.Nodes, config.Node{
				Address: node.Address(AdvertizeAddress, "http", config.PortOffsetTendermintP2P, config.PortOffsetDirectory),
				Type:    node.DnnType,
			})

			bvnn := mkcfg(network.Id, config.BlockValidator, node.BvnnType, bvn.Id)
			bvnn.Moniker = fmt.Sprintf("%s.%d", bvn.Id, j+1)
			ConfigureNodePorts(node, bvnn, config.PortOffsetBlockValidator)
			bvnConfig.Nodes = append(bvnConfig.Nodes, config.Node{
				Address: node.Address(AdvertizeAddress, "http", config.PortOffsetTendermintP2P, config.PortOffsetBlockValidator),
				Type:    node.BvnnType,
			})

			if dnn.P2P.ExternalAddress == "" {
				dnn.P2P.ExternalAddress = dnn.Accumulate.LocalAddress
			}
			if bvnn.P2P.ExternalAddress == "" {
				bvnn.P2P.ExternalAddress = bvnn.Accumulate.LocalAddress
			}

			if len(network.Bvns) == 1 && len(bvn.Nodes) == 1 {
				dnn.P2P.AddrBookStrict = true
				dnn.P2P.AllowDuplicateIP = false
			} else {
				dnn.P2P.AddrBookStrict = false
				dnn.P2P.AllowDuplicateIP = true
				dnn.P2P.PersistentPeers = strings.Join(network.Peers(node, config.PortOffsetTendermintP2P, config.PortOffsetDirectory), ",")
			}

			if len(bvn.Nodes) == 1 {
				bvnn.P2P.AddrBookStrict = true
				bvnn.P2P.AllowDuplicateIP = false
			} else {
				bvnn.P2P.AddrBookStrict = false
				bvnn.P2P.AllowDuplicateIP = true
				bvnn.P2P.PersistentPeers = strings.Join(bvn.Peers(node, config.PortOffsetTendermintP2P, config.PortOffsetBlockValidator), ",")
			}

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

func ConfigureNodePorts(node *NodeInit, cfg *config.Config, offset config.PortOffset) {
	cfg.P2P.ListenAddress = node.Address(ListenAddress, "tcp", offset, config.PortOffsetTendermintP2P)
	cfg.RPC.ListenAddress = node.Address(ListenAddress, "tcp", offset, config.PortOffsetTendermintRpc)

	cfg.Instrumentation.PrometheusListenAddr = fmt.Sprintf(":%d", node.Port(offset, config.PortOffsetPrometheus))
	if cfg.Accumulate.LocalAddress == "" {
		cfg.Accumulate.LocalAddress = node.Address(AdvertizeAddress, "", offset, config.PortOffsetTendermintP2P)
	}
	cfg.Accumulate.Website.ListenAddress = node.Address(ListenAddress, "http", offset, config.PortOffsetWebsite)
	cfg.Accumulate.API.ListenAddress = node.Address(ListenAddress, "http", offset, config.PortOffsetAccumulateApi)
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
			key := ed25519.PrivKey(node.PrivValKey)
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
		netType := config.BlockValidator
		if id == protocol.Directory {
			netType = config.Directory
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

func loadOrCreatePrivVal(config *config.Config, key []byte) error {
	keyFile := config.PrivValidator.KeyFile()
	stateFile := config.PrivValidator.StateFile()
	if !tmos.FileExists(keyFile) {
		pv := privval.NewFilePV(ed25519.PrivKey(key), keyFile, stateFile)
		pv.Save()
		return nil
	}
	var pv *privval.FilePV
	var err error
	if !tmos.FileExists(stateFile) {
		// When initializing the other node, the key file has already been created
		pv = privval.NewFilePV(ed25519.PrivKey(key), keyFile, stateFile)
		pv.LastSignState.Save()
		// Don't return here - we still need to check that the key on disk matches what we expect
	} else { // if file exists then we need to load it
		pv, err = privval.LoadFilePV(keyFile, stateFile)
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
		nodeKey := tmtypes.NodeKey{
			ID:      tmtypes.NodeIDFromPubKey(ed25519.PubKey(key[32:])),
			PrivKey: ed25519.PrivKey(key),
		}
		return nodeKey.SaveAs(keyFile)
	}

	nodeKey, err := tmtypes.LoadNodeKey(keyFile)
	if err != nil {
		return err
	}

	if !bytes.Equal(nodeKey.PrivKey.Bytes(), key) {
		return fmt.Errorf("existing private key does not match try using --reset flag")
	}

	return nil
}

func LoadOrGenerateTmPrivKey(privFileName string) (ed25519.PrivKey, error) {
	//attempt to load the priv validator key, create otherwise.
	b, err := ioutil.ReadFile(privFileName)
	var privValKey ed25519.PrivKey
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			//do not overwrite a private validator key.
			return ed25519.GenPrivKey(), nil
		}
		return nil, err
	}
	var pvkey privval.FilePVKey
	err = tmjson.Unmarshal(b, &pvkey)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal existing private validator from %s: %v try using --reset flag", privFileName, err)
	} else {
		privValKey = pvkey.PrivKey.(ed25519.PrivKey)
	}

	return privValKey, nil
}

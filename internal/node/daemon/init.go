// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cometbft/cometbft/crypto/ed25519"
	tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
	tmjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/libs/log"
	tmos "github.com/cometbft/cometbft/libs/os"
	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/privval"
	tmtypes "github.com/cometbft/cometbft/types"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/genesis"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const nodeDirPerm = 0755

type MakeConfigFunc func(networkName string, net protocol.PartitionType, node config.NodeType, netId string) *config.Config

func BuildNodesConfig(network *NetworkInit, mkcfg MakeConfigFunc) [][][]*config.Config {
	var allConfigs [][][]*config.Config

	if mkcfg == nil {
		mkcfg = config.Default
	}

	dnConfig := config.Partition{
		Id:       protocol.Directory,
		Type:     protocol.PartitionTypeDirectory,
		BasePort: int64(network.Bvns[0].Nodes[0].BasePort), // TODO This is not great
	}
	bsnConfig := config.Partition{
		Type:     protocol.PartitionTypeBlockSummary,
		BasePort: int64(network.Bvns[0].Nodes[0].BasePort) + int64(config.PortOffsetBlockSummary), // TODO This is not great
	}
	if network.Bsn != nil {
		bsnConfig.Id = network.Bsn.Id
	}

	// If the node addresses are loopback or private IPs, disable strict address book
	ip := net.ParseIP(network.Bvns[0].Nodes[0].Peer().String())
	strict := ip == nil || !(ip.IsLoopback() || ip.IsPrivate())

	var i int
	for _, bvn := range network.Bvns {
		var bvnConfigs [][]*config.Config
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

			dnn.Instrumentation.Namespace += fmt.Sprintf("_directory_%d", i)
			bvnn.Instrumentation.Namespace += fmt.Sprintf("_%s_%d", strings.ToLower(bvn.Id), i)

			if dnn.P2P.ExternalAddress == "" {
				dnn.P2P.ExternalAddress = node.Peer().TendermintP2P().Directory().String()
			}
			if bvnn.P2P.ExternalAddress == "" {
				bvnn.P2P.ExternalAddress = node.Peer().TendermintP2P().BlockValidator().String()
			}

			if network.Bsn != nil {
				dnn.Accumulate.SummaryNetwork = network.Bsn.Id
				bvnn.Accumulate.SummaryNetwork = network.Bsn.Id
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

			dnn.Accumulate.P2P.BootstrapPeers = accumulate.BootstrapServers
			bvnn.Accumulate.P2P.BootstrapPeers = accumulate.BootstrapServers

			if network.Bootstrap != nil {
				p2pPeers := AddressSliceBuilder{network.Bootstrap.Peer()}.
					AccumulateP2P().WithKey().PartitionType(protocol.PartitionTypeBootstrap).
					Do(func(b AddressBuilder) AddressBuilder { return b.Scheme("tcp") }, func(b AddressBuilder) AddressBuilder { return b.Scheme("udp") }).
					Multiaddr()
				dnn.Accumulate.P2P.BootstrapPeers = p2pPeers
				bvnn.Accumulate.P2P.BootstrapPeers = p2pPeers
			}

			bvnConfigs = append(bvnConfigs, []*config.Config{dnn, bvnn})
		}
		allConfigs = append(allConfigs, bvnConfigs)
	}

	if network.Bsn != nil {
		var bsnConfigs [][]*config.Config
		for i, node := range network.Bsn.Nodes {
			bsnn := mkcfg(network.Id, protocol.PartitionTypeBlockSummary, node.BsnnType, network.Bsn.Id)
			bsnn.Moniker = fmt.Sprintf("%s.%d", network.Bsn.Id, i+1)
			ConfigureNodePorts(node, bsnn, protocol.PartitionTypeBlockSummary)
			bsnConfig.Nodes = append(bsnConfig.Nodes, config.Node{
				Address: node.Advertize().Scheme("http").TendermintP2P().BlockSummary().String(),
				Type:    node.BsnnType,
			})

			if bsnn.P2P.ExternalAddress == "" {
				bsnn.P2P.ExternalAddress = node.Peer().TendermintP2P().BlockSummary().String()
			}

			// No duplicate IPs
			bsnn.P2P.AllowDuplicateIP = false

			// Initial peers (should be bootstrap peers but that setting isn't
			// present in 0.37)
			bsnn.P2P.PersistentPeers = strings.Join(network.Bsn.Peers(node).BlockSummary().TendermintP2P().WithKey().String(), ",")

			// Set whether unroutable addresses are allowed
			bsnn.P2P.AddrBookStrict = strict

			p2pPeers := network.Peers(node).AccumulateP2P().WithKey().
				Do(AddressBuilder.Directory, AddressBuilder.BlockValidator).
				Do(func(b AddressBuilder) AddressBuilder { return b.Scheme("tcp") }, func(b AddressBuilder) AddressBuilder { return b.Scheme("udp") }).
				Multiaddr()
			bsnn.Accumulate.P2P.BootstrapPeers = p2pPeers

			bsnConfigs = append(bsnConfigs, []*config.Config{bsnn})
		}
		allConfigs = append(allConfigs, bsnConfigs)
	}

	if network.Bootstrap != nil {
		cfg := config.Default(network.Id, protocol.PartitionTypeBootstrap, 0, "")
		ConfigureNodePorts(network.Bootstrap, cfg, protocol.PartitionTypeDirectory)
		cfg.Accumulate.P2P.BootstrapPeers = nil
		cfg.Accumulate.AnalysisLog = config.AnalysisLog{}
		cfg.Accumulate.Snapshots = config.Snapshots{}
		cfg.Storage = nil

		allConfigs = append(allConfigs, [][]*config.Config{{cfg}})
	}

	return allConfigs
}

func ConfigureNodePorts(node *NodeInit, cfg *config.Config, part protocol.PartitionType) {
	cfg.P2P.ListenAddress = node.Listen().Scheme("tcp").PartitionType(part).TendermintP2P().String()
	cfg.RPC.ListenAddress = node.Listen().Scheme("tcp").PartitionType(part).TendermintRPC().String()

	cfg.Instrumentation.PrometheusListenAddr = node.Listen().PartitionType(part).Prometheus().String()
	cfg.Accumulate.P2P.Listen = []multiaddr.Multiaddr{
		node.Listen().Scheme("tcp").PartitionType(part).AccumulateP2P().Multiaddr(),
		node.Listen().Scheme("udp").PartitionType(part).AccumulateP2P().Multiaddr(),
	}
	cfg.Accumulate.API.ListenAddress = node.Listen().Scheme("http").PartitionType(part).AccumulateAPI().String()
}

func BuildGenesisDocs(network *NetworkInit, globals *core.GlobalValues, time time.Time, logger log.Logger, factomAddresses func() (io.Reader, error), snapshots []func(*core.GlobalValues) (ioutil2.SectionReader, error)) (map[string][]byte, error) {
	docs := map[string][]byte{}
	var operators [][]byte
	netinfo := new(protocol.NetworkDefinition)
	netinfo.NetworkName = network.Id
	netinfo.AddPartition(protocol.Directory, protocol.PartitionTypeDirectory)

	var i int
	for _, bvn := range network.Bvns {
		netinfo.AddPartition(bvn.Id, protocol.PartitionTypeBlockValidator)

		for _, node := range bvn.Nodes {
			i++
			key := tmed25519.PrivKey(node.PrivValKey)
			operators = append(operators, key.PubKey().Bytes())
			netinfo.AddValidator(key.PubKey().Bytes(), protocol.Directory, node.DnnType == config.Validator)
			netinfo.AddValidator(key.PubKey().Bytes(), bvn.Id, node.BvnnType == config.Validator)
		}
	}

	if network.Bsn != nil {
		for _, node := range network.Bsn.Nodes {
			key := tmed25519.PrivKey(node.PrivValKey)
			operators = append(operators, key.PubKey().Bytes())
			netinfo.AddValidator(key.PubKey().Bytes(), network.Bsn.Id, node.BsnnType == config.Validator)
		}
	}

	globals.Network = netinfo

	ids := []string{protocol.Directory}
	for _, bvn := range network.Bvns {
		ids = append(ids, bvn.Id)
	}

	bsnSnapBuf := new(ioutil2.Buffer)
	var bsnSnap *snapshot.Writer
	var err error
	if network.Bsn != nil {
		header := new(snapshot.Header)
		header.Height = 1
		header.Timestamp = time
		header.PartitionSnapshotIDs = ids
		bsnSnap, err = snapshot.Create(bsnSnapBuf, header)
		if err != nil {
			return nil, err
		}
	}

	for _, id := range ids {
		netType := protocol.PartitionTypeBlockValidator
		if strings.EqualFold(id, protocol.Directory) {
			netType = protocol.PartitionTypeDirectory
		}
		snapBuf := new(ioutil2.Buffer)
		err = genesis.Init(snapBuf, genesis.InitOpts{
			NetworkID:       network.Id,
			PartitionId:     id,
			NetworkType:     netType,
			GenesisTime:     time,
			Logger:          logger.With("partition", id),
			GenesisGlobals:  globals,
			OperatorKeys:    operators,
			FactomAddresses: factomAddresses,
			Snapshots:       snapshots,
			ConsensusParams: tmtypes.DefaultConsensusParams(),
		})
		if err != nil {
			return nil, err
		}

		// Write the snapshot to the BSN snapshot
		if network.Bsn != nil {
			w, err := bsnSnap.Open(snapshot.SectionTypeSnapshot)
			if err != nil {
				return nil, err
			}
			_, err = bytes.NewBuffer(snapBuf.Bytes()).WriteTo(w)
			if err != nil {
				return nil, err
			}
			err = w.Close()
			if err != nil {
				return nil, err
			}
		}

		docs[id] = snapBuf.Bytes()
	}

	if network.Bsn != nil {
		docs[network.Bsn.Id] = bsnSnapBuf.Bytes()
	}

	return docs, nil
}

func WriteNodeFiles(cfg *config.Config, privValKey, nodeKey []byte, genDoc []byte) (err error) {
	defer func() {
		if err != nil {
			_ = os.RemoveAll(cfg.RootDir)
		}
	}()

	// Bootstrap node
	if cfg.Accumulate.NetworkType == protocol.PartitionTypeBootstrap {
		cfg.NodeKey = "node_key.json"

		err = os.MkdirAll(cfg.RootDir, nodeDirPerm)
		if err != nil {
			return fmt.Errorf("failed to create config dir: %v", err)
		}

		err = config.StoreAcc(cfg, cfg.RootDir)
		if err != nil {
			return fmt.Errorf("failed to write config files: %w", err)
		}

		err = loadOrCreateNodeKey(cfg, nodeKey)
		if err != nil {
			return fmt.Errorf("failed to write node key: %w", err)
		}
		return nil
	}

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

	err = os.WriteFile(cfg.GenesisFile(), genDoc, 0644)
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

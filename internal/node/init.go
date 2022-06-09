package node

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	tmcfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/libs/log"
	tmlog "github.com/tendermint/tendermint/libs/log"
	tmos "github.com/tendermint/tendermint/libs/os"
	tmtime "github.com/tendermint/tendermint/libs/time"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/types"
	cfg "gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/genesis"
	"gitlab.com/accumulatenetwork/accumulate/networks"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

const nodeDirPerm = 0755

type InitOptions struct {
	Version             int
	WorkDir             string
	NodeDirNames        []string
	Port                int
	GenesisDoc          *types.GenesisDoc
	Config              []*cfg.Config
	RemoteIP            []string
	ListenIP            []string
	ValidatorKeys       []crypto.PrivKey
	NetworkValidatorMap genesis.NetworkValidatorMap
	Logger              log.Logger
	FactomAddressesFile string
}

// Init creates the initial configuration for a set of nodes, using
// the given configuration. Config, remoteIP, and opts.ListenIP must all be of equal
// length.
func Init(opts InitOptions) (bootstrap genesis.Bootstrap, err error) {
	switch opts.Version {
	case 0:
		fallthrough
	case 1:
		bootstrap, err = initV1(opts)
	case 2:
		//todo: err = initV2(opts)
	default:
		return nil, fmt.Errorf("unknown version to init")
	}
	return bootstrap, err
}

// initV1 creates the initial configuration for a set of nodes, using
// the given configuration. Config, remoteIP, and opts.ListenIP must all be of equal
// length.
func initV1(opts InitOptions) (bootstrap genesis.Bootstrap, err error) {
	defer func() {
		if err != nil {
			_ = os.RemoveAll(opts.WorkDir)
		}
	}()

	if opts.Logger != nil {
		opts.Logger.Info("Tendermint initialize")
	}

	configs := opts.Config
	subnetID := configs[0].Accumulate.Network.LocalSubnetID
	genVals := make([]types.GenesisValidator, 0, len(configs))
	genValKeys := make([][]byte, 0, len(configs))

	var networkType cfg.NetworkType
	var ports []int
	for i, config := range configs {
		if i == 0 {
			networkType = config.Accumulate.Network.Type
		} else if config.Accumulate.Network.Type != networkType {
			return nil, errors.New("Cannot initialize multiple networks at once")
		}

		var nodeDirName string
		if len(opts.NodeDirNames) > 0 {
			nodeDirName = opts.NodeDirNames[i]
		} else {
			nodeDirName = fmt.Sprintf("Node%d", i)
		}
		nodeDir := path.Join(opts.WorkDir, nodeDirName)
		config.SetRoot(nodeDir)

		port := opts.Port
		if port == 0 {
			bits := strings.Split(config.Accumulate.Network.LocalAddress, ":")
			if len(bits) != 2 {
				return nil, fmt.Errorf("invalid local address %q", config.Accumulate.Network.LocalAddress)
			}
			i, err := strconv.ParseInt(bits[1], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid local address %q: %w", config.Accumulate.Network.LocalAddress, err)
			}
			port = int(i)
		}
		ports = append(ports, port)

		config.P2P.ListenAddress = fmt.Sprintf("tcp://%s:%d", opts.ListenIP[i], port+networks.TmP2pPortOffset)
		config.RPC.ListenAddress = fmt.Sprintf("tcp://%s:%d", opts.ListenIP[i], port+networks.TmRpcPortOffset)
		config.RPC.GRPCListenAddress = fmt.Sprintf("tcp://%s:%d", opts.ListenIP[i], port+networks.TmRpcGrpcPortOffset)
		config.Instrumentation.PrometheusListenAddr = fmt.Sprintf(":%d", port+networks.TmPrometheusPortOffset)

		err = os.MkdirAll(path.Join(nodeDir, "config"), nodeDirPerm)
		if err != nil {
			return nil, fmt.Errorf("failed to create config dir: %v", err)
		}

		err = os.MkdirAll(path.Join(nodeDir, "data"), nodeDirPerm)
		if err != nil {
			return nil, fmt.Errorf("failed to create data dir: %v", err)
		}

		var pk crypto.PrivKey
		if opts.ValidatorKeys != nil {
			pk = opts.ValidatorKeys[i]
		}
		if err := initFilesWithConfig(config, pk, &subnetID, opts.GenesisDoc); err != nil {
			return nil, err
		}

		pvKeyFile := path.Join(nodeDir, config.PrivValidator.Key)
		pvStateFile := path.Join(nodeDir, config.PrivValidator.State)
		pv, err := privval.LoadFilePV(pvKeyFile, pvStateFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load private validator: %v", err)
		}

		if config.Mode == tmcfg.ModeValidator {
			genVals = append(genVals, types.GenesisValidator{
				Address: pv.Key.Address,
				PubKey:  pv.Key.PubKey,
				Power:   1,
				Name:    nodeDirName,
			})
			genValKeys = append(genValKeys, pv.Key.PrivKey.Bytes())
		}
	}

	// Generate genesis doc from generated validators
	if opts.GenesisDoc == nil {
		genTime := tmtime.Now()

		db := memory.New(opts.Logger.With("module", "storage"))
		bootstrap, err = genesis.Init(db, genesis.InitOpts{
			Network:             configs[0].Accumulate.Network,
			AllConfigs:          configs,
			GenesisTime:         genTime,
			Validators:          genVals,
			Keys:                genValKeys,
			NetworkValidatorMap: opts.NetworkValidatorMap,
			Logger:              opts.Logger,
			FactomAddressesFile: opts.FactomAddressesFile,
		})
		if err != nil {
			return nil, err
		}
	}

	// Gather validator peer addresses.
	validatorPeers := map[int]string{}
	for i, config := range configs {
		// if config.Mode != tmcfg.ModeValidator {
		// 	continue
		// }

		nodeKey, err := types.LoadNodeKey(config.NodeKeyFile())
		if err != nil {
			return nil, fmt.Errorf("failed to load node key: %v", err)
		}
		validatorPeers[i] = nodeKey.ID.AddressString(fmt.Sprintf("%s:%d", opts.RemoteIP[i], ports[i]))
	}

	// Overwrite default config.
	nConfig := len(configs)
	for i, config := range configs {
		if nConfig > 1 {
			config.P2P.AddrBookStrict = false
			config.P2P.AllowDuplicateIP = true
			config.P2P.PersistentPeers = ""
			if config.P2P.PersistentPeers == "" {
				for j, peer := range validatorPeers {
					if j != i {
						config.P2P.PersistentPeers += "," + peer
					}
				}
				// Remove the leading comma
				config.P2P.PersistentPeers = config.P2P.PersistentPeers[1:]
			}
		} else {
			config.P2P.AddrBookStrict = true
			config.P2P.AllowDuplicateIP = false
		}
		config.Moniker = fmt.Sprintf("%s.%d", config.Accumulate.Network.LocalSubnetID, i)

		config.Accumulate.Website.ListenAddress = fmt.Sprintf("http://%s:8080", opts.ListenIP[i])
		config.Accumulate.API.ListenAddress = fmt.Sprintf("http://%s:%d", opts.ListenIP[i], ports[i]+networks.AccApiPortOffset)

		if strings.Contains(config.P2P.PersistentPeers, "26756") && subnetID != "Directory" {
			print("")
		}
		err := cfg.Store(config)
		if err != nil {
			return nil, err
		}
	}

	logMsg := []interface{}{"module", "init"}
	switch nValidators := len(genVals); nValidators {
	case 0:
		logMsg = append(logMsg, "followers", nConfig)
	case nConfig:
		logMsg = append(logMsg, "validators", nConfig)
	default:
		logMsg = append(logMsg, "validators", nValidators, "followers", nConfig-nValidators)
	}
	opts.Logger.Info("Successfully initialized nodes", logMsg...)
	return bootstrap, nil
}

func initFilesWithConfig(config *cfg.Config, privKey crypto.PrivKey, chainid *string, genDoc *types.GenesisDoc) error {

	logger := tmlog.NewNopLogger()

	// private validator
	privValKeyFile := config.PrivValidator.KeyFile()
	privValStateFile := config.PrivValidator.StateFile()
	var pv *privval.FilePV
	var err error
	if tmos.FileExists(privValKeyFile) {
		pv, err = privval.LoadFilePV(privValKeyFile, privValStateFile)
		if err != nil {
			return fmt.Errorf("failed to load private validator: %w", err)
		}
		logger.Info("Found private validator", "keyFile", privValKeyFile,
			"stateFile", privValStateFile)
	} else if privKey != nil {
		pv = privval.NewFilePV(privKey, privValKeyFile, privValStateFile)
		pv.Save()
		logger.Info("Wrote private validator", "keyFile", privValKeyFile,
			"stateFile", privValStateFile)
	} else {
		pv, err = privval.GenFilePV(privValKeyFile, privValStateFile, "")
		if err != nil {
			return fmt.Errorf("failed to gen private validator: %w", err)
		}
		pv.Save()
		logger.Info("Generated private validator", "keyFile", privValKeyFile,
			"stateFile", privValStateFile)
	}

	nodeKeyFile := config.NodeKeyFile()
	if tmos.FileExists(nodeKeyFile) {
		logger.Info("Found node key", "path", nodeKeyFile)
	} else {
		if _, err := types.LoadOrGenNodeKey(nodeKeyFile); err != nil {
			return fmt.Errorf("can't load or gen node key: %v", err)
		}
		logger.Info("Generated node key", "path", nodeKeyFile)
	}
	// genesis file
	genFile := config.GenesisFile()
	if tmos.FileExists(genFile) {
		logger.Info("Found genesis file", "path", genFile)
	} else {
		if genDoc == nil {
			genDoc := types.GenesisDoc{
				ChainID:         *chainid,
				GenesisTime:     tmtime.Now(),
				ConsensusParams: types.DefaultConsensusParams(),
			}
			pubKey, err := pv.GetPubKey(context.Background())
			if err != nil {
				return fmt.Errorf("can't get pubkey: %v", err)
			}
			genDoc.Validators = []types.GenesisValidator{{
				Address: pubKey.Address(),
				PubKey:  pubKey,
				Power:   10,
			}}
		}

		if err := genDoc.SaveAs(genFile); err != nil {
			return fmt.Errorf("can't save genFile: %s: %v", genFile, err)
		}
		logger.Info("Generated genesis file", "path", genFile)
	}

	return nil
}

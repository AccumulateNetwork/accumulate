package tendermint

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/AccumulateNetwork/accumulated/networks"

	"github.com/spf13/viper"

	"time"

	abcicli "github.com/tendermint/tendermint/abci/client"
	abciserver "github.com/tendermint/tendermint/abci/server"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	cfg "github.com/tendermint/tendermint/config"
	tmlog "github.com/tendermint/tendermint/libs/log"
	tmos "github.com/tendermint/tendermint/libs/os"
	tmrand "github.com/tendermint/tendermint/libs/rand"
	"github.com/tendermint/tendermint/libs/service"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	coregrpc "github.com/tendermint/tendermint/rpc/grpc"
	rpcclient "github.com/tendermint/tendermint/rpc/jsonrpc/client"
	"github.com/tendermint/tendermint/types"
	tmtime "github.com/tendermint/tendermint/types/time"
)

const (
	nodeDirPerm = 0755
)

func Initialize(shardname string, index int, WorkingDir string) {
	var nValidators int
	var defNodeName string
	var localAddress string

	localAddress = "tcp://0.0.0.0"
	defNodeName = "Node"
	fmt.Println("Tendermint Initialize")

	nValidators = len(networks.Networks[index].Ip)
	config := cfg.DefaultConfig()

	genVals := make([]types.GenesisValidator, nValidators)

	for i := 0; i < nValidators; i++ {
		nodeDirName := fmt.Sprintf("%s%d", defNodeName, i)
		nodeDir := path.Join(WorkingDir, nodeDirName)
		config.SetRoot(nodeDir)
		config.Instrumentation.Namespace = shardname
		config.ProxyApp = fmt.Sprintf("%s:%d", localAddress, networks.Networks[index].Port)
		config.RPC.ListenAddress = fmt.Sprintf("%s:%d", localAddress, networks.Networks[index].Port+1)
		config.RPC.GRPCListenAddress = fmt.Sprintf("%s:%d", localAddress, networks.Networks[index].Port+2)
		if nValidators > 1 {
			config.P2P.ListenAddress = fmt.Sprintf("%s:%d", localAddress, networks.Networks[index].Port)
		}
		config.Instrumentation.PrometheusListenAddr = fmt.Sprintf(":%d", networks.Networks[index].Port)
		//	   config.Consensus.CreateEmptyBlocks = false
		err := os.MkdirAll(path.Join(nodeDir, "config"), nodeDirPerm)
		if err != nil {
			_ = os.RemoveAll(WorkingDir)
			fmt.Printf("Can't make config directory: %s/config\n", nodeDir)
			return
		}

		err = os.MkdirAll(path.Join(nodeDir, "data"), nodeDirPerm)
		if err != nil {
			_ = os.RemoveAll(WorkingDir)
			fmt.Printf("Can't make data directory: %s/data\n", nodeDir)
			return
		}

		if err := initFilesWithConfig(config, &networks.Networks[index].Name); err != nil {
			fmt.Printf("Init Files with Config failed\n")
			return
		}

		pvKeyFile := path.Join(nodeDir, config.BaseConfig.PrivValidatorKey)
		pvStateFile := path.Join(nodeDir, config.BaseConfig.PrivValidatorState)
		pv := privval.LoadFilePV(pvKeyFile, pvStateFile)

		pubKey, err := pv.GetPubKey()
		if err != nil {
			fmt.Printf("can't get pubkey: %w\n", err)
			return
		}
		genVals[i] = types.GenesisValidator{
			Address: pubKey.Address(),
			PubKey:  pubKey,
			Power:   1,
			Name:    nodeDirName,
		}
	}
	// Generate genesis doc from generated validators
	genDoc := &types.GenesisDoc{
		ChainID:         "chain-" + tmrand.Str(6),
		GenesisTime:     tmtime.Now(),
		InitialHeight:   0,
		Validators:      genVals,
		ConsensusParams: types.DefaultConsensusParams(),
	}
	// Write genesis file.
	for i := 0; i < nValidators; i++ {
		nodeDir := path.Join(WorkingDir, fmt.Sprintf("%s%d", defNodeName, i))
		if err := genDoc.SaveAs(path.Join(nodeDir, config.BaseConfig.Genesis)); err != nil {
			_ = os.RemoveAll(WorkingDir)
			fmt.Printf("Can't save gen doc file %s/%s\n", nodeDir, config.BaseConfig.Genesis)
			return
		}
	}
	// Gather persistent peer addresses.

	persistentPeers := make([]string, nValidators)

	IPs := networks.Networks[index].Ip

	for i := 1; i < nValidators; i++ {
		nodeDir := path.Join(WorkingDir, fmt.Sprintf("%s%d", defNodeName, i))
		config.SetRoot(nodeDir)
		nodeKey, err := p2p.LoadNodeKey(config.NodeKeyFile())
		if err != nil {
			_ = os.RemoveAll(WorkingDir)
			fmt.Printf("Can't load node key ID\n")
			return
		}
		persistentPeers[i] = p2p.IDAddressString(nodeKey.ID(), fmt.Sprintf("%s:%d", IPs[i], networks.Networks[index].Port))
	}

	// Overwrite default config.
	for i := 0; i < nValidators; i++ {
		nodeDir := path.Join(WorkingDir, fmt.Sprintf("%s%d", defNodeName, i))
		config.SetRoot(nodeDir)
		config.LogLevel = "main:info,state:info,statesync:info,*:error"
		if nValidators > 1 {
			config.P2P.AddrBookStrict = false
			config.P2P.AllowDuplicateIP = true
			config.P2P.PersistentPeers = strings.Join(persistentPeers, ",")
		} else {
			config.P2P.AddrBookStrict = true
			config.P2P.AllowDuplicateIP = false
		}
		config.Moniker = fmt.Sprintf("%s%d", defNodeName, i)

		ConfigFile := nodeDir + "/config/config.toml"

		cfg.WriteConfigFile(ConfigFile, config)

		v := viper.New()
		v.SetConfigFile(ConfigFile)
		v.ReadInConfig()

		// accRCPAddress
		addr := fmt.Sprintf("%s:%d", localAddress, networks.Networks[index].Port+3)
		v.Set("accumulate.AccRPCAddress", addr)

		// routerAddress
		addr = fmt.Sprintf("%s:%d", localAddress, networks.Networks[index].Port+4)
		v.Set("accumulate.RouterAddress", addr)

		v.WriteConfig()
	}

	fmt.Printf("Successfully initialized %v node directories\n", nValidators)

	return
}

func initFilesWithConfig(config *cfg.Config, chainid *string) error {

	logger := tmlog.NewNopLogger()

	// private validator
	privValKeyFile := config.PrivValidatorKeyFile()
	privValStateFile := config.PrivValidatorStateFile()
	var pv *privval.FilePV
	if tmos.FileExists(privValKeyFile) {
		pv = privval.LoadFilePV(privValKeyFile, privValStateFile)
		logger.Info("Found private validator", "keyFile", privValKeyFile,
			"stateFile", privValStateFile)
	} else {
		pv = privval.GenFilePV(privValKeyFile, privValStateFile)
		pv.Save()
		logger.Info("Generated private validator", "keyFile", privValKeyFile,
			"stateFile", privValStateFile)
	}

	nodeKeyFile := config.NodeKeyFile()
	if tmos.FileExists(nodeKeyFile) {
		logger.Info("Found node key", "path", nodeKeyFile)
	} else {
		if _, err := p2p.LoadOrGenNodeKey(nodeKeyFile); err != nil {
			fmt.Printf("Can't load or gen node key\n")
			return err
		}
		logger.Info("Generated node key", "path", nodeKeyFile)
	}
	// genesis file
	genFile := config.GenesisFile()
	if tmos.FileExists(genFile) {
		logger.Info("Found genesis file", "path", genFile)
	} else {
		genDoc := types.GenesisDoc{
			ChainID:         *chainid,
			GenesisTime:     tmtime.Now(),
			ConsensusParams: types.DefaultConsensusParams(),
		}
		pubKey, err := pv.GetPubKey()
		if err != nil {
			fmt.Printf("can't get pubkey: %w\n", err)
			return err
		}
		genDoc.Validators = []types.GenesisValidator{{
			Address: pubKey.Address(),
			PubKey:  pubKey,
			Power:   10,
		}}

		if err := genDoc.SaveAs(genFile); err != nil {
			fmt.Printf("Can't save genFile: %s\n", genFile)
			return err
		}
		logger.Info("Generated genesis file", "path", genFile)
	}

	return nil
}

func makeGRPCClient(addr string) (abcicli.Client, error) { //grpccore.BroadcastAPIClient, error) {//abcicli.Client, error) {
	// Start the listener
	socket := addr                 //fmt.Sprintf("unix://%s.sock", addr)
	logger := tmlog.NewNopLogger() //TestingLogger()

	//client := grpccore.StartGRPCClient(addr)
	client := abcicli.NewGRPCClient(socket, true)

	client.SetLogger(logger.With("module", "abci-client"))
	if err := client.Start(); err != nil {
		return nil, err
	}
	return client, nil
}

//
//func makeRPCClient(addr string) rpcclient.ABCIClient {
//}
//

func makeGRPCServer(app abcitypes.Application, name string) (service.Service, error) {
	// Start the listener
	socket := name                 // fmt.Sprintf("unix://%s.sock", name)
	logger := tmlog.NewNopLogger() //TestingLogger()

	gapp := abcitypes.NewGRPCApplication(app)
	server := abciserver.NewGRPCServer(socket, gapp)
	server.SetLogger(logger.With("module", "abci-server"))
	if err := server.Start(); err != nil {
		return nil, err
	}

	return server, nil
}
func WaitForRPC(laddr string) {
	//laddr := GetConfig().RPC.ListenAddress
	client, err := rpcclient.New(laddr)
	if err != nil {
		panic(err)
	}
	result := new(ctypes.ResultStatus)
	for {
		_, err := client.Call(context.Background(), "status", map[string]interface{}{}, result)
		if err == nil {
			return
		}

		// fmt.Println("error", err)
		time.Sleep(time.Millisecond)
	}
}
func GetGRPCClient(grpcAddr string) coregrpc.BroadcastAPIClient {
	return coregrpc.StartGRPCClient(grpcAddr)
}

func GetRPCClient(rpcAddr string) *rpcclient.Client {
	client, _ := rpcclient.New(rpcAddr)
	//b := client.NewRequestBatch()
	//
	//result := new(ctypes.ResultStatus)
	//_, err := client.Call(context.Background(), "status", map[string]interface{}{}, result)
	//b.Call()
	return client
}

func WaitForGRPC(grpcAddr string) {
	client := GetGRPCClient(grpcAddr)
	for {
		_, err := client.Ping(context.Background(), &coregrpc.RequestPing{})
		if err == nil {
			return
		}
	}
}

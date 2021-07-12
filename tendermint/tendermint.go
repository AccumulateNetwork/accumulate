package tendermint

import (
	"fmt"
	"github.com/spf13/viper"
	abcicli "github.com/tendermint/tendermint/abci/client"
	abciserver "github.com/tendermint/tendermint/abci/server"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	cfg "github.com/tendermint/tendermint/config"
	config "github.com/tendermint/tendermint/config"
	tmlog "github.com/tendermint/tendermint/libs/log"
	tmos "github.com/tendermint/tendermint/libs/os"
	"github.com/tendermint/tendermint/libs/service"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"

	"github.com/tendermint/tendermint/types"
	tmtime "github.com/tendermint/tendermint/types/time"
)

func Initialize(shardname string,
	ABCIAppAddress string, RPCAddress string, GRPCAddress string, AccRPCAddress string, RouterAddress string,
	ConfigFile string, WorkingDir string){
	fmt.Println("Tendermint Initialize")
	config.EnsureRoot(WorkingDir)
	var newConfig = cfg.DefaultConfig()
	newConfig.SetRoot(WorkingDir)
	//newConfig.BaseConfig.
	newConfig.Instrumentation.Namespace = shardname
	newConfig.ProxyApp = ABCIAppAddress
	newConfig.RPC.ListenAddress = RPCAddress
	newConfig.RPC.GRPCListenAddress = GRPCAddress
	config.WriteConfigFile(ConfigFile,newConfig)

	v := viper.New()
	v.SetConfigFile(ConfigFile)
	v.AddConfigPath(WorkingDir)
	v.ReadInConfig()
	type config struct {
		Port int
		Name string
	}
	v.Set("accumulate.AccRPCAddress",AccRPCAddress)
	v.Set("accumulate.RouterAddress", RouterAddress)
	v.WriteConfig()

	if InitFilesWithConfig(newConfig,&shardname) != nil {
		//log.Fatal("")
		return
	}
}

func InitFilesWithConfig(config *cfg.Config, chainid *string) error {
	// private validator
	privValKeyFile := config.PrivValidatorKeyFile()
	privValStateFile := config.PrivValidatorStateFile()
	var pv *privval.FilePV
	if tmos.FileExists(privValKeyFile) {
		pv = privval.LoadFilePV(privValKeyFile, privValStateFile)
		//		logger.Info("Found private validator", "keyFile", privValKeyFile,
		//			"stateFile", privValStateFile)
	} else {
		pv = privval.GenFilePV(privValKeyFile, privValStateFile)
		pv.Save()
		//		logger.Info("Generated private validator", "keyFile", privValKeyFile,
		//			"stateFile", privValStateFile)
	}

	nodeKeyFile := config.NodeKeyFile()
	if tmos.FileExists(nodeKeyFile) {
		//		logger.Info("Found node key", "path", nodeKeyFile)
	} else {
		if _, err := p2p.LoadOrGenNodeKey(nodeKeyFile); err != nil {
			return err
		}
		//		logger.Info("Generated node key", "path", nodeKeyFile)
		fmt.Println("Generated node key", "path", nodeKeyFile)
	}

	// genesis file
	genFile := config.GenesisFile()
	if tmos.FileExists(genFile) {
		//		logger.Info("Found genesis file", "path", genFile)
		fmt.Println("Found genesis file", "path", genFile)
	} else {
		genDoc := types.GenesisDoc{
			ChainID:         *chainid, //fmt.Sprintf("test-chain-%v", tmrand.Str(6)),
			GenesisTime:     tmtime.Now(),
			ConsensusParams: types.DefaultConsensusParams(),
		}
		pubKey, err := pv.GetPubKey()
		if err != nil {
			return fmt.Errorf("can't get pubkey: %w", err)
		}
		genDoc.Validators = []types.GenesisValidator{{
			Address: pubKey.Address(),
			PubKey:  pubKey,
			Power:   10,
		}}

		if err := genDoc.SaveAs(genFile); err != nil {
			return err
		}
		//	logger.Info("Generated genesis file", "path", genFile)
		fmt.Println("Generated genesis file", "path", genFile)
	}

	return nil
}

func makeGRPCClient(addr string) (abcicli.Client,error){ //grpccore.BroadcastAPIClient, error) {//abcicli.Client, error) {
	// Start the listener
	socket := addr //fmt.Sprintf("unix://%s.sock", addr)
	logger := tmlog.NewNopLogger()//TestingLogger()

	//client := grpccore.StartGRPCClient(addr)
	client := abcicli.NewGRPCClient(socket, true)

	client.SetLogger(logger.With("module", "abci-client"))
	if err := client.Start(); err != nil {
		return nil, err
	}
	return client, nil
}

func makeGRPCServer(app abcitypes.Application, name string) (service.Service, error) {
	// Start the listener
	socket := name// fmt.Sprintf("unix://%s.sock", name)
	logger := tmlog.NewNopLogger()//TestingLogger()

	gapp := abcitypes.NewGRPCApplication(app)
	server := abciserver.NewGRPCServer(socket, gapp)
	server.SetLogger(logger.With("module", "abci-server"))
	if err := server.Start(); err != nil {
		return nil, err
	}

	return server, nil
}

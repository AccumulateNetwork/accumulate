package tendermint

import (
	"fmt"
	abcicli "github.com/tendermint/tendermint/abci/client"
	abciserver "github.com/tendermint/tendermint/abci/server"
	"github.com/tendermint/tendermint/abci/types"
	cfg "github.com/tendermint/tendermint/config"
	config "github.com/tendermint/tendermint/config"
	tmlog "github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/service"
)

func Initialize(chainid string, ABCIAppAddress string, RPCAddress string, ConfigFile string, WorkingDir string){
	fmt.Println("Tendermint Initialize")
	config.EnsureRoot(WorkingDir)
	var newConfig = cfg.DefaultConfig()
	newConfig.SetRoot(WorkingDir)
	//newConfig.BaseConfig.
	newConfig.Instrumentation.Namespace = chainid
	newConfig.ProxyApp = ABCIAppAddress
	newConfig.RPC.ListenAddress = RPCAddress
	config.WriteConfigFile(ConfigFile,newConfig)
	if InitFilesWithConfig(newConfig,&chainid) != nil {
		//log.Fatal("")
		return
	}
}

func makeGRPCClient(addr string) (abcicli.Client, error) {
	// Start the listener
	socket := addr //fmt.Sprintf("unix://%s.sock", addr)
	logger := tmlog.NewNopLogger()//TestingLogger()

	client := abcicli.NewGRPCClient(socket, true)
	client.SetLogger(logger.With("module", "abci-client"))
	if err := client.Start(); err != nil {
		return nil, err
	}
	return client, nil
}

func makeGRPCServer(app types.Application, name string) (service.Service, error) {
	// Start the listener
	socket := name// fmt.Sprintf("unix://%s.sock", name)
	logger := tmlog.NewNopLogger()//TestingLogger()

	gapp := types.NewGRPCApplication(app)
	server := abciserver.NewGRPCServer(socket, gapp)
	server.SetLogger(logger.With("module", "abci-server"))
	if err := server.Start(); err != nil {
		return nil, err
	}

	return server, nil
}
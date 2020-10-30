package tendermint

import (
	"fmt"
	cfg "github.com/tendermint/tendermint/config"
	config "github.com/tendermint/tendermint/config"
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
	InitFilesWithConfig(newConfig,&chainid)
}



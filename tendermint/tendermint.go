package tendermint

import (
	"fmt"
	cfg "github.com/tendermint/tendermint/config"
	config "github.com/tendermint/tendermint/config"
)

func Initialize(Namespace string, ABCIAppAddress string, RPCAddress string, ConfigFile string, WorkingDir string){
	fmt.Println("Tendermint Initialize")
	config.EnsureRoot(WorkingDir)
	var newConfig = cfg.DefaultConfig()
	newConfig.SetRoot(WorkingDir)
	newConfig.Instrumentation.Namespace = Namespace
	newConfig.ProxyApp = ABCIAppAddress
	newConfig.RPC.ListenAddress = RPCAddress
	config.WriteConfigFile(ConfigFile,newConfig)
	InitFilesWithConfig(newConfig)
}



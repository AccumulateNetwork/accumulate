package router

import (
	"github.com/AccumulateNetwork/accumulated/blockchain/accumulate"
	"github.com/AccumulateNetwork/accumulated/blockchain/tendermint"

	"github.com/spf13/viper"

	tmnet "github.com/tendermint/tendermint/libs/net"
	//"github.com/tendermint/tendermint/rpc/client/local"
)

func RandPort() int {
	port, err := tmnet.GetFreePort()
	if err != nil {
		panic(err)
	}
	return port
}

func boostrapBVC(configfile string, workingdir string, baseport int) error {

	//ABCIAddress := fmt.Sprintf("tcp://localhost:%d", baseport)
	//RPCAddress := fmt.Sprintf("tcp://localhost:%d", baseport+1)
	//GRPCAddress := fmt.Sprintf("tcp://localhost:%d", baseport+2)
	//
	//AccRPCInternalAddress := fmt.Sprintf("tcp://localhost:%d", baseport+3) //no longer needed
	//
	//RouterPublicAddress := fmt.Sprintf("tcp://localhost:%d", baseport+4)

	tendermint.Initialize("accumulate.", 2, workingdir)
	//create the default configuration files for the blockchain.
	//tendedrmint.Initialize("accumulate.routertest", ABCIAddress, RPCAddress, GRPCAddress,
	//	AccRPCInternalAddress, RouterPublicAddress, configfile, workingdir)

	viper.SetConfigFile(configfile)
	viper.AddConfigPath(workingdir + "/Node0")
	//[mempool]
	//	broadcast = true
	//	cache_size = 100000
	//	max_batch_bytes = 10485760
	//	max_tx_bytes = 1048576
	//	max_txs_bytes = 1073741824
	//	recheck = true
	//	size = 50000
	//	wal_dir = ""
	//

	//viper.Set("mempool.keep-invalid-txs-in-cache, "false"
	//viper.Set("mempool.max_txs_bytes", "1073741824")
	viper.Set("mempool.max_batch_bytes", 1048576)
	viper.Set("mempool.cache_size", 1048576)
	viper.Set("mempool.size", 50000)
	err := viper.WriteConfig()
	if err != nil {
		panic(err)
	}

	return nil
}

func makeBVC(configfile string, workingdir string) *tendermint.AccumulatorVMApplication {
	app, err := accumulate.CreateAccumulateBVC(configfile, workingdir)
	if err != nil {
		panic(err)
	}
	return app
}

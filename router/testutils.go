package router

import (
	"github.com/AccumulateNetwork/accumulated/blockchain/accumulate"
	"github.com/AccumulateNetwork/accumulated/blockchain/tendermint"
	"github.com/spf13/viper"
	tmnet "github.com/tendermint/tendermint/libs/net"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	"github.com/tendermint/tendermint/rpc/client/local"
)

func RandPort() int {
	port, err := tmnet.GetFreePort()
	if err != nil {
		panic(err)
	}
	return port
}

func boostrapBVC(configfile string, workingdir string, baseport int) error {
	tendermint.Initialize("accumulate.", 2, workingdir)
	viper.SetConfigFile(configfile)
	viper.AddConfigPath(workingdir + "/Node0")
	viper.ReadInConfig()
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

func makeBVCandRouter(cfg string, dir string) (*local.Local, *rpchttp.HTTP, *tendermint.AccumulatorVMApplication) {

	//Select a base port to open.  Ports 43210, 43211, 43212, 43213,43214 need to be open
	baseport := 43210

	//generate the config files needed to run a test BVC
	err := boostrapBVC(cfg, dir, baseport)
	if err != nil {
		panic(err)
	}

	//First we need to build a Router.  The router has to be done first since the BVC connects to it.
	//Make the router's client (i.e. Public facing GRPC client that will route the message to the correct network) and
	//server (i.e. The GRPC that will convert public GRPC messages into messages to communicate with the BVC application)
	viper.SetConfigFile(cfg)
	viper.AddConfigPath(dir)
	viper.ReadInConfig()

	///Build a BVC we'll use for our test
	accvm := makeBVC(cfg, dir+"/Node0")

	lc, _ := accvm.GetLocalClient()
	laddr := viper.GetString("rpc.laddr")
	//rpcc := tendermint.GetRPCClient(laddr)

	rpcc, _ := rpchttp.New(laddr, "/websocket")

	return &lc, rpcc, accvm
}

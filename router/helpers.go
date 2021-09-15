package router

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/blockchain/accumulate"
	"github.com/AccumulateNetwork/accumulated/blockchain/tendermint"

	//	"github.com/AccumulateNetwork/accumulated/blockchain/accnode"
	//	"github.com/AccumulateNetwork/accumulated/blockchain/tendermint"
	"github.com/AccumulateNetwork/accumulated/types/proto"
	"github.com/spf13/viper"

	tmnet "github.com/tendermint/tendermint/libs/net"
	"github.com/tendermint/tendermint/rpc/client/local"

	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	//"github.com/tendermint/tendermint/rpc/client/local"
	"google.golang.org/grpc"
)

func RandPort() int {
	port, err := tmnet.GetFreePort()
	if err != nil {
		panic(err)
	}
	return port
}

func makeApiServiceClientAndServer(routeraddress string) (proto.ApiServiceClient, *RouterConfig, error) {

	r := NewRouter(routeraddress)

	if r == nil {
		return nil, nil, fmt.Errorf("failed to create router")
	}

	conn, err := grpc.Dial(routeraddress, grpc.WithInsecure(), grpc.WithContextDialer(dialerFunc))
	if err != nil {
		return nil, nil, fmt.Errorf("error openning GRPC client in router")
	}

	client := proto.NewApiServiceClient(conn)

	return client, r, nil
}

func makeClientAndServer(routeraddress string) (proto.ApiServiceClient, *RouterConfig) {

	r := NewRouter(routeraddress)

	if r == nil {
		panic("Failed to create router")
	}
	conn, err := grpc.Dial(routeraddress, grpc.WithInsecure(), grpc.WithContextDialer(dialerFunc))
	if err != nil {
		panic("Error Openning GRPC client in router")
	}

	client := proto.NewApiServiceClient(conn)

	return client, r
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

	//viper.Set("mempool.max_batch_bytes", "10485760")
	//viper.Set("mempool.max_txs_bytes", "1073741824")
	viper.Set("mempool.cache_size", "1000000")
	viper.Set("mempool.size", "50000")
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

func makeBVCandRouter(cfg string, dir string) (proto.ApiServiceClient, *RouterConfig, *local.Local, *rpchttp.HTTP, *tendermint.AccumulatorVMApplication) {

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
	routeraddress := viper.GetString("accumulate.RouterAddress")
	client, routerserver := makeClientAndServer(routeraddress)

	///Build a BVC we'll use for our test
	accvm := makeBVC(cfg, dir+"/Node0")

	//This will register the Tendermint RPC client of the BVC with the router
	accvmapi, _ := accvm.GetAPIClient()

	//this needs to go away and go through a discovery process instead
	routerserver.AddBVCClient(accvm.GetName(), accvmapi)

	lc, _ := accvm.GetLocalClient()
	laddr := viper.GetString("rpc.laddr")
	//rpcc := tendermint.GetRPCClient(laddr)

	rpcc, _ := rpchttp.New(laddr, "/websocket")

	return client, routerserver, &lc, rpcc, accvm
}

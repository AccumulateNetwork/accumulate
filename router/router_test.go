package router

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/AccumulateNetwork/SMT/managed"
	"github.com/AccumulateNetwork/accumulated/blockchain/accnode"
	"github.com/AccumulateNetwork/accumulated/blockchain/tendermint"
	"github.com/AccumulateNetwork/accumulated/blockchain/validator"
	"github.com/spf13/viper"
	"io/ioutil"
	"os"
	"path"

	//"fmt"
	"github.com/AccumulateNetwork/accumulated/api/proto"
	"github.com/golang/protobuf/ptypes/empty"

	//"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"testing"
)

func makeClientAndServer(t *testing.T, routeraddress string) (proto.ApiServiceClient, *RouterConfig) {


	r := NewRouter(routeraddress)

	if r == nil {
		t.Fatal("Failed to create router")
	}
	conn, err := grpc.Dial(routeraddress, grpc.WithInsecure(), grpc.WithContextDialer(dialerFunc))
	if err != nil {
		t.Fatalf("Error Openning GRPC client in router")
	}

	client := proto.NewApiServiceClient(conn)
	return client, r
}
func boostrapBVC(t *testing.T, configfile string, workingdir string, baseport int) error {

	ABCIAddress := fmt.Sprintf("tcp://localhost:%d",baseport)
	RPCAddress := fmt.Sprintf("tcp://localhost:%d",baseport+1)
	GRPCAddress := fmt.Sprintf("tcp://localhost:%d",baseport+2)

	AccRPCInternalAddress := fmt.Sprintf("tcp://localhost:%d",baseport+3)
	RouterPublicAddress := fmt.Sprintf("tcp://localhost:%d",baseport+4)

	//create the default configuration files for the blockchain.
	tendermint.Initialize("accumulate.routertest", ABCIAddress,RPCAddress,GRPCAddress,
		AccRPCInternalAddress,RouterPublicAddress,configfile,workingdir)

	return nil
}

func makeBVC(t *testing.T, configfile string, workingdir string) *tendermint.AccumulatorVMApplication {
	app:= accnode.CreateAccumulateBVC(configfile,workingdir)
	return app
}

func makeBVCandRouter(t *testing.T, cfg string, dir string) (proto.ApiServiceClient, *RouterConfig ) {



	//Select a base port to open.  Ports 43210, 43211, 43212, 43213,43214 need to be open
	baseport := 43210

	//generate the config files needed to run a test BVC
	err := boostrapBVC(t, cfg,dir,baseport)
	if err != nil {
		t.Fatal(err)
	}

	//First we need to build a Router.  The router has to be done first since the BVC connects to it.
	//Make the router's client (i.e. Public facing GRPC client that will route the message to the correct network) and
	//server (i.e. The GRPC that will convert public GRPC messages into messages to communicate with the BVC application)
	viper.SetConfigFile(cfg)
	viper.AddConfigPath(dir)
	viper.ReadInConfig()
	routeraddress := viper.GetString("accumulate.RouterAddress")
	client,routerserver := makeClientAndServer(t,routeraddress)

	///Build a BVC we'll use for our test
	accvm := makeBVC(t,cfg, dir)


	//This will register the Tendermint RPC client of the BVC with the router
	accvmapi, _ := accvm.GetAPIClient()

	//this needs to go away and go through a discovery process instead
	routerserver.AddBVCClient(accvm.GetName(), accvmapi)

	return client,routerserver
}
//func testClient(t *testing.T, routeraddress string) {
//
//	client := makeClient(t, routeraddress)
//	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
//	defer cancel()
//
//
//	sub := proto.Submission{}
//	sub.Type = 1234
//
//	res, err := client.ProcessTx(ctx, &sub)
//
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	if res.ErrorCode != 0x9000 {
//		t.Fatalf("Error code not 0x9000, transaction failed with error code %d", res.ErrorCode)
//	}
//}

func TestURL(t *testing.T) {

	//create a URL with invalid utf8

	//create a URL without acc://

	//create a URL with sub account Wagon
	//Red is primary, and Wagon is secondary.
	urlstring := "acc://RedWagon/acc"
	q,err := URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}

	urlstring = "acc://RedWagon/acc?block=1000"
	q,err = URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}

	if q.Query != "{\"block\":[\"1000\"]}" {
		t.Fatalf("URL query failed:  expected block=1000 received %s", q.Query)
	}

	urlstring = "acc://RedWagon/acc?currentblock&block=1000+index"
	q,err = URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}

    urlstring = "acc://RedWagon/identity?replace=PUBLICKEY1_HEX+PUBLICKEY2_HEX&signature=f97a65de43"
	q,err = URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}

	urlstring = "acc://RedWagon?replace=PUBLICKEY1_HEX+PUBLICKEY2_HEX&signature=f97a65de43"
	q,err = URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRouter(t *testing.T) {
	//routeraddress := "tcp://localhost:54321"

	//_, r := makeClientAndServer(t,routeraddress)

	//create a client connection to the router.
    //testClient(t, routeraddress)

    //Router()

    //r.Close()
}

/// CreateIdentity acc://RedWagon

/// * Who signs the identity?  Identities need to be bootstrapped. I.e. Someone needs to pay for it...
/// * Need to assign it to an initial public key?
func TestQuery(t *testing.T) {

	//make a temporary director to configure a test BVC
	dir, err := ioutil.TempDir("/tmp", "AccRouterTest-")
	cfg := path.Join(dir, "/config/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	client, routerserver := makeBVCandRouter(t,cfg,dir)
	defer routerserver.Close()

	//pointless call to do some quick URL tests.
	Router()

	//Test BVC Count call
	e := empty.Empty{}
	_, err = client.QueryShardCount(context.Background(), &e)
	if err != nil {
		t.Fatalf("Error sending query for shard count")
	}


	//Test Query
	//urlstring := "acc://RedWagon/acc/query=block"

	urlstring := "acc://RedWagon?key&prority=1"

	q,err := URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}

	res, err := client.Query(context.Background(),&q)
	if err != nil {
		t.Fatalf("Error sending query for shard count")
	}
	fmt.Printf("URL Querty Test string %s result: %d\n",urlstring , res.Code)

    sub := proto.Submission{}
    sub.Address = validator.GetTypeIdFromName()
    sub.Chainid = managed.Hash{} //just submit what you want


}


func hexToBytes(hexStr string) []byte {
	raw, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}
	return raw
}


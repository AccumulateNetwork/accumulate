package router

import (
	"context"
	"encoding/hex"
	"fmt"

	//"fmt"
	"github.com/AccumulateNetwork/accumulated/proto"
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
func TestCreateIdentity(t *testing.T) {
	routeraddress := "tcp://localhost:54321"

	urlstring := "acc://RedWagon/acc/query=balance"


	client,r := makeClientAndServer(t,routeraddress)

	Router()

	e := empty.Empty{}
	_,err := client.QueryShardCount(context.Background(),&e)
	if err != nil {
		t.Fatalf("Error sending query for shard count")
	}

	q := URLParser(urlstring)
	res, _ := client.Query(context.Background(),&q)
	fmt.Printf("URL Querty Test string %s result: %d\n",urlstring , res.Code)

    r.Close()
}

func hexToBytes(hexStr string) []byte {
	raw, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}
	return raw
}


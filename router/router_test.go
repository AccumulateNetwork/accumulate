package router

import (
	"context"
	"encoding/hex"
	"github.com/AccumulateNetwork/accumulated/proto"
	"google.golang.org/grpc"
	"testing"
	"time"
)

func makeClient(t *testing.T, routeraddress string) proto.ApiServiceClient {

	conn, err := grpc.Dial(routeraddress, grpc.WithInsecure(), grpc.WithContextDialer(dialerFunc))
	if err != nil {
		t.Fatalf("Error Openning GRPC client in router")
	}
	defer conn.Close()
	client := proto.NewApiServiceClient(conn)
	return client
}

func testClient(t *testing.T, routeraddress string) {

	client := makeClient(t, routeraddress)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()


	sub := proto.Submission{}
	sub.Type = 1234

	res, err := client.SyntheticTx(ctx, &sub)

	if err != nil {
		t.Fatal(err)
	}

	if res.ErrorCode != 0x9000 {
		t.Fatalf("Error code not 0x9000, transaction failed with error code %d", res.ErrorCode)
	}
}

func TestRouter(t *testing.T) {
	routeraddress := "tcp://localhost:54321"
	//routeraddress := "unix://router_testsocket"

	r := NewRouter(routeraddress)

	if r == nil {
		t.Fatal("Failed to create router")
	}

	//create a client connection to the router.
    //testClient(t, routeraddress)

    Router()


}

func hexToBytes(hexStr string) []byte {
	raw, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}
	return raw
}


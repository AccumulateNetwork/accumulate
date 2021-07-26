package router

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/AdamSLevy/jsonrpc2/v14"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"testing"
	"time"
)

func ackcheck(t *testing.T, url string) {

	type Params struct {
		Hash    *string `json:"hash"`
		Chainid *string `json:"chainid"`
	}

	hash := sha256.Sum256([]byte("test"))
	hashstring := hex.EncodeToString(hash[:])
	chainid := sha256.Sum256([]byte("chaintest"))
	chainidstring := hex.EncodeToString(chainid[:])
	r := Params{&hashstring, &chainidstring}

	type Data struct {
		Status string `json:"status"`
	}
	type CommitAck struct {
		Committxid string `json:"committxid"`
		Entryhash  string `json:"entryhash"`
		Commitdata Data   `json:"commitdata"`
	}

	type EntryResponse struct {
		Result    CommitAck `json:"result"`
		Entrydata Data      `json:"entrydata"`
	}

	var res EntryResponse

	//token resposne
	//{
	//	"jsonrpc":"2.0",
	//	"id":0,
	//	"result":{
	//	"txid":"f1d9919829fa71ce18caf1bd8659cce8a06c0026d3f3fffc61054ebb25ebeaa0",
	//		"transactiondate":1441138021975,
	//		"transactiondatestring":"2015-09-01 15:07:01",
	//		"blockdate":1441137600000,
	//		"blockdatestring":"2015-09-01 15:00:00",
	//		"status":"DBlockConfirmed"
	//}
	//

	var rpc jsonrpc2.Client
	err := rpc.Request(context.Background(), url, "ack", r, res)

	if err != nil {
		t.Fatal(err)
	}

}

type T struct {
	FactoidSubmit       interface{} `json:"factoid-submit"`
	AblockByHeight      interface{} `json:"ablock-by-height"`
	Ack                 interface{} `json:"ack"`
	CommitChain         interface{} `json:"commit-chain"`
	CommitEntry         interface{} `json:"commit-entry"`
	Entry               interface{} `json:"entry"`
	TokenBalance        interface{} `json:"token-balance"`
	PendingTransactions interface{} `json:"pending-transactions"`
	RevealChain         interface{} `json:"reveal-chain"`
	RevealEntry         interface{} `json:"reveal-entry"`
	CommitRevealEntry   interface{} `json:"commit-reveal-entry"`
	Transaction         interface{} `json:"transaction"`
}

func TestJsonrpcserver2(t *testing.T) {
	//make a temporary director to configure a test BVC
	dir, err := ioutil.TempDir("/tmp", "AccRouterTest-")
	cfg := path.Join(dir, "/config/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	_, routerserver, _, rpcc := makeBVCandRouter(t, cfg, dir)
	defer routerserver.Close()

	testport := RandPort()

	Jsonrpcserver2(rpcc, testport)

	url := "http://localhost:" + strconv.Itoa(testport)
	ackcheck(t, url)

	time.Sleep(20 * time.Second)

}

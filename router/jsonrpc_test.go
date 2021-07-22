package router

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/AdamSLevy/jsonrpc2/v14"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"
)

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

	Jsonrpcserver2(rpcc, 1234)

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

	type eret struct {
		Result    CommitAck `json:"result"`
		Entrydata Data      `json:"entrydata"`
	}

	var res eret
	var rpc jsonrpc2.Client
	err = rpc.Request(context.Background(), "http://localhost:1234", "ack",
		r, res)

	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(20 * time.Second)

}

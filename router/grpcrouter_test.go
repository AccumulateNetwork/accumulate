package router

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/AccumulateNetwork/accumulated/blockchain/validator/types"
	"github.com/Factom-Asset-Tokens/fatd/fat0"
	proto1 "github.com/golang/protobuf/proto"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"time"

	//"fmt"
	"github.com/AccumulateNetwork/accumulated/api/proto"
	"github.com/golang/protobuf/ptypes/empty"

	rpchttp "github.com/tendermint/tendermint/rpc/client/http"

	"testing"
)

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

func CreateKeyPair() ed25519.PrivKey {
	return ed25519.GenPrivKey()
}

func createIdentity(t *testing.T) *proto.Submission {
	kp := CreateKeyPair()
	sponsor := CreateKeyPair() //this really needs to be some existing identity of who ever is paying for it.

	name := "RedWagon"
	sub, err := CreateIdentityTest(&name, kp.PubKey().(ed25519.PubKey), sponsor)
	if err != nil {
		t.Fatalf("Error creating submission %v", err)
	}
	return sub
}

func createTransaction(t *testing.T) *proto.Submission {
	sub := proto.Submission{}

	sub.Identitychain = types.GetIdentityChainFromAdi("RedWagon").Bytes()
	sub.Chainid = types.GetIdentityChainFromAdi("RedWagon/acc").Bytes()
	sub.Type = 0 //this is going away it is not needed since we'll know the type from transaction
	sub.Instruction = proto.AccInstruction_Token_Transaction

	//transaction := `{"inputs":{"FA3tM2R3T2ZT2gPrTfxjqhnFsdiqQUyKboKxvka3z5c1JF9yQck5":100,"FA3tM2R3T2ZT2gPrTfxjqhnFsdiqQUyKboKxvka3z5c1JF9yQck5":100,"FA3rCRnpU95ieYCwh7YGH99YUWPjdVEjk73mpjqnVpTDt3rUUhX8":10},"metadata":[0],"outputs":{"FA1zT4aFpEvcnPqPCigB3fvGu4Q4mTXY22iiuV69DqE1pNhdF2MC":10,"FA3sjgNF4hrJAiD9tQxAVjWS9Ca1hMqyxtuVSZTBqJiPwD7bnHkn":90,"FA2uyZviB3vs28VkqkfnhoXRD8XdKP1zaq7iukq2gBfCq3hxeuE8":10}}`
	transaction := `{"inputs":{"RedWagon/acc":100},"outputs":{"GreenRock":10,"BlueRock":90}}`
	tx := fat0.Transaction{}
	tx.UnmarshalJSON([]byte(transaction))

	var err error
	sub.Data, err = tx.Entry.MarshalBinary()

	if err != nil {
		t.Fatal(err)
	}
	kp := CreateKeyPair()
	sub.Signature, err = kp.Sign(sub.Data)
	if err != nil {
		return nil
	}
	sub.Key = kp.PubKey().Bytes()

	return &sub
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
	client, routerserver, lc, rpcc := makeBVCandRouter(t, cfg, dir)
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

	q, err := URLParser(urlstring)
	if err != nil {
		t.Fatal(err)
	}

	res, err := client.ProcessLightQuery(context.Background(), q)
	if err != nil {
		t.Fatalf("Error sending query for shard count")
	}
	fmt.Printf("URL Querty Test string %s result: %d\n", urlstring, res.Code)

	sub := createIdentity(t)

	time.Sleep(4 * time.Second)
	for i := 0; i < 1; i++ {
		sub.Param1 = rand.Uint64()
		//var tx tmtypes.Tx
		//x := tx.(tmtypes.Tx)
		x, _ := proto1.Marshal(sub)
		_, err := lc.BroadcastTxAsync(context.Background(), x)
		if err != nil {
			i--
			continue
		}
		//client.ProcessTx(context.Background(),sub)
	}

	time.Sleep(4 * time.Second)

	// Create a new batch
	txs := make([][]byte, 5000)
	batches := make([]*rpchttp.BatchHTTP, 20)
	for j, batch := range batches {
		batch = rpcc.NewBatch()
		batches[j] = batch
		for i := 0; i < 1000; i++ {
			sub.Param1 = rand.Uint64()
			tx, _ := proto1.Marshal(sub)
			txs[i] = tx
		}
		for _, tx := range txs {
			if _, err := batch.BroadcastTxAsync(context.Background(), tx); err != nil {
				t.Fatal(err) //nolint:gocritic
			}
		}
	}

	// Send the batch of 2 transactions
	for i, batch := range batches {
		//fmt.Printf("Sending batch %d of %d\n", i, len(batches))
		time.Sleep(200 * time.Millisecond)
		if _, err := batch.Send(context.Background()); err != nil {
			for err != nil {
				fmt.Printf("Resending batch %d of %d\n", i, len(batches))
				_, err = batch.Send(context.Background())
			}
			//t.Fatal(err)
		}
	}

	//idres, err := client.ProcessTx(context.Background(),sub)

	//if err != nil {
	//	t.Fatal(err)
	//}
	//
	//if idres.ErrorCode != 0 {
	//	t.Fatalf("%X err: %v", idres.Respdata, err)
	//}

	//sub := proto.Submission{}
	//sub.Identitychain = validator.()
	//sub.Chainid = managed.Hash{} //just submit what you want

	time.Sleep(20 * time.Second)

}

func hexToBytes(hexStr string) []byte {
	raw, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}
	return raw
}

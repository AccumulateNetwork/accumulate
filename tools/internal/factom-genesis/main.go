package factom

import (
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	tmjson "github.com/tendermint/tendermint/libs/json"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"

	f2 "github.com/FactomProject/factom"
	"github.com/tendermint/tendermint/privval"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/cmd"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var factomChainData map[[32]byte]*Queue

var origin *url.URL
var key *cmd.Key
var simul *simulator.Simulator
var delivered = (*protocol.TransactionStatus).Delivered

type simTb struct{}

func (simTb) Name() string         { return "Simulator" }
func (simTb) Log(a ...interface{}) { fmt.Println(a...) } //nolint
func (simTb) Fail()                {}                    // The simulator never exits so why record anything?
func (simTb) FailNow()             { os.Exit(1) }
func (simTb) Helper()              {}

const (
	LOCAL_URL = "http://127.0.1.1:26660"
)

func InitSim() {
	// Initialize
	sim := simulator.New(simTb{}, 3)
	simul = sim
	simul.InitFromGenesis()
}

func SetPrivateKeyAndOrigin(privateKey string) error {
	b, err := ioutil.ReadFile(privateKey)
	if err != nil {
		return err
	}
	var pvkey privval.FilePVKey
	var pub, priv []byte
	err = tmjson.Unmarshal(b, &pvkey)
	if err != nil {
		return err
	}
	if pvkey.PubKey != nil {
		pub = pvkey.PubKey.Bytes()
	}
	if pvkey.PrivKey != nil {
		priv = pvkey.PrivKey.Bytes()
	}

	key = &cmd.Key{PrivateKey: priv, PublicKey: pub, Type: protocol.SignatureTypeED25519}
	//url, err := url.Parse("acc://bvn-BVN1.acme")
	//if err != nil {
	//	log.Fatalf("Error : ", err.Error())
	//}
	url, err := protocol.LiteTokenAddress(key.PublicKey, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		log.Fatalf("cannot create lite token account %v", err)
	}
	log.Println("URL : ", url)

	origin = url
	return nil
}

func buildEnvelope(payload protocol.TransactionBody, originUrl *url.URL) (*protocol.Envelope, error) {
	txn := new(protocol.Transaction)
	txn.Body = payload
	txn.Header.Principal = originUrl
	signer := new(signing.Builder)
	signer.SetPrivateKey(key.PrivateKey)
	signer.SetTimestampToNow()
	signer.SetVersion(1)
	signer.SetType(protocol.SignatureTypeED25519)
	signer.SetUrl(originUrl)

	sig, err := signer.Initiate(txn)
	if err != nil {
		log.Println("Error : ", err.Error())
		return nil, err
	}

	envelope := new(protocol.Envelope)
	envelope.Transaction = append(envelope.Transaction, txn)
	envelope.Signatures = append(envelope.Signatures, sig)
	envelope.TxHash = append(envelope.TxHash, txn.GetHash()...)

	return envelope, nil
}

func WriteDataToAccumulateSim(data protocol.DataEntry, dataAccount *url.URL) error {
	log.Println("Writing to : ", dataAccount.String())
	wd := &protocol.WriteDataTo{
		Entry:     &protocol.AccumulateDataEntry{Data: data.GetData()},
		Recipient: dataAccount,
	}

	timestamp, _ := signing.TimestampFromValue(time.Now().UTC().UnixMilli()).Get()
	log.Println("Executing txn")
	responses, _ := simul.WaitForTransactions(delivered, simul.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(origin).
			WithTimestampVar(&timestamp).
			WithSigner(origin, 1).
			WithBody(wd).
			Initiate(protocol.SignatureTypeED25519, key.PrivateKey).
			Build())...)
	for _, res := range responses {
		log.Println("Response : ", res)
	}
	log.Println("Wrote to : ", dataAccount.String())
	return nil
}

var mutex sync.Mutex

func WriteDataToAccumulate(env string, data protocol.DataEntry, dataAccount *url.URL) error {
	client, err := client.New(env)
	defer client.CloseIdleConnections()
	if err != nil {
		log.Println("Error : ", err.Error())
		return err
	}
	queryRes, err := queryDataByHash(client, dataAccount, data.Hash())
	if err == nil && queryRes.Data != nil {
		log.Printf("====== %x, %x", queryRes.ChainId, data.Hash())
		err := fmt.Errorf("record for data entry hash is already available")
		return err
	}

	wd := &protocol.WriteDataTo{
		Entry:     &protocol.AccumulateDataEntry{Data: data.GetData()},
		Recipient: dataAccount,
	}

	//need to have a mutex here to keep timestamps sequential for nonce check since we are using same principal.
	mutex.Lock()
	envelope, err := buildEnvelope(wd, origin)
	if err != nil {
		mutex.Unlock()
		return err
	}

	req := new(api.ExecuteRequest)
	req.Envelope = envelope

	res, err := client.ExecuteDirect(context.Background(), req)
	mutex.Unlock()
	if err != nil {
		log.Printf("%v", err)
		return err
	}
	if res.Code != 0 {
		log.Printf("Response Error : %v txid: %x code: %d", res.Message, res.TransactionHash, res.Code)
		return fmt.Errorf(res.Message)
	}

	txReq := api.TxnQuery{}
	txReq.Txid = res.TransactionHash
	txReq.Wait = time.Second * 10
	txReq.IgnorePending = false

	queryResTx, err := client.QueryTx(context.Background(), &txReq)
	if err != nil {
		return err
	}

	for _, txid := range queryResTx.Produced {
		txReq.Txid = txid.Account().Hash()
		_, err = client.QueryTx(context.Background(), &txReq)
		if err != nil {
			return err
		}
	}

	//
	retries := 10
	success := false
	for i := 0; i < retries; i++ {
		log.Println("Retry : ", i)
		queryRes, err = queryDataByHash(client, dataAccount, data.Hash())
		if err != nil {
			log.Printf("attempt %d error (%x): %v\n", i, data.Hash(), err)
			continue
		}
		success = true
		break
	}
	if !success {
		return fmt.Errorf("read back failed %v", err)
	}
	log.Println("Success : ", queryResTx.Txid.Account(), data.Hash())
	return nil
}

func queryDataByHash(client *client.Client, account *url.URL, hash []byte) (*api.ChainQueryResponse, error) {
	queryReq := &api.DataEntryQuery{
		Url:       account,
		EntryHash: *(*[32]byte)(hash),
	}
	return client.QueryData(context.Background(), queryReq)
}

func WriteDataFromQueueToAccumulate(env string) {
	for chainId, data := range factomChainData {
		// go ExecuteQueueToWriteData(chainId, data)
		chainUrl, err := protocol.LiteDataAddress(chainId[:]) //nolint:rangevarref
		if err != nil {
			log.Println("Error : ", err.Error())
			break
		}

		log.Printf("Writing data to %s", chainUrl.String())
		ExecuteQueueToWriteData(env, chainUrl, data)
	}
}

type ChainGang struct {
	mapChannels map[[32]byte]chan *protocol.FactomDataEntry
	Wait        sync.WaitGroup
}

func (w *ChainGang) Close() {
	w.Wait.Wait()
	for _, v := range w.mapChannels {
		close(v)
	}
}

func (w *ChainGang) GetOrCreateChainWorker(s string, chainId *[32]byte, maxEntries int) chan *protocol.FactomDataEntry {
	v, ok := w.mapChannels[*chainId]
	if !ok {
		v = make(chan *protocol.FactomDataEntry, maxEntries)
		u, err := protocol.LiteDataAddress((*chainId)[:])
		if err != nil {
			log.Fatalf("error creating lite address %x, %v", *chainId, err)
		}
		go w.WriteDataWorker(s, u, v)
		// w.WriteDataWorker(s, u, v)
	}
	return v
}

func ExecuteDataEntry(chainId *[32]byte, entry *protocol.FactomDataEntry) {
	u, err := protocol.LiteDataAddress((*chainId)[:])
	if err != nil {
		log.Fatalf("error creating lite address %x, %v", *chainId, err)
	}
	WriteDataToSim(u, entry)
}

func (w *ChainGang) WriteDataWorker(env string, chainUrl *url.URL, queue chan *protocol.FactomDataEntry) {
	w.Wait.Add(1)
	defer w.Wait.Done()
	for entry := range queue {
		err := WriteDataToAccumulateSim(entry, chainUrl)
		if err != nil {
			log.Printf("error writing data to accumulate : %v", err)
		}
	}

}

func WriteDataToSim(chainUrl *url.URL, entry *protocol.FactomDataEntry) {
	err := WriteDataToAccumulateSim(entry, chainUrl)
	if err != nil {
		log.Printf("error writing data to accumulate : %v", err)
	}
}

func ExecuteQueueToWriteData(env string, chainUrl *url.URL, queue *Queue) {
	for {
		if len(queue.q) > 0 {
			entry := queue.Pop().(*f2.Entry)
			dataEntry := ConvertFactomDataEntryToLiteDataEntry(*entry)
			err := WriteDataToAccumulate(env, dataEntry, chainUrl)
			if err != nil {
				log.Println("Error writing data to accumulate : ", err.Error())
			}
		} else {
			break
		}
	}
}

func GetAccountFromPrivateString(hexString string) *url.URL {
	var key cmd.Key
	privKey, err := hex.DecodeString(hexString)
	if err == nil && len(privKey) == 64 {
		key.PrivateKey = privKey
		key.PublicKey = privKey[32:]
		key.Type = protocol.SignatureTypeED25519
	}
	return protocol.LiteAuthorityForKey(key.PublicKey, key.Type)
}

func ConvertFactomDataEntryToLiteDataEntry(entry f2.Entry) *protocol.FactomDataEntry {
	dataEntry := new(protocol.FactomDataEntry)
	chainId, err := hex.DecodeString(entry.ChainID)
	if err != nil {
		log.Printf(" Error: invalid chainId ")
		return nil
	}
	copy(dataEntry.AccountId[:], chainId)
	dataEntry.Data = []byte(entry.Content)
	dataEntry.ExtIds = entry.ExtIDs
	return dataEntry
}

func GetDataAndPopulateQueue(entries []*f2.Entry) {
	factomChainData = make(map[[32]byte]*Queue)
	for _, entry := range entries {
		accountId, err := hex.DecodeString(entry.ChainID)
		if err != nil {
			log.Fatalf("cannot decode account id")
		}
		_, ok := factomChainData[*(*[32]byte)(accountId)]
		if !ok {
			factomChainData[*(*[32]byte)(accountId)] = NewQueue()
		}
		factomChainData[*(*[32]byte)(accountId)].Push(entry)
	}
}

//FaucetWithCredits is only used for testing. Initial account will be prefunded.
func FaucetWithCredits() error {
	simul.CreateAccount(&protocol.LiteIdentity{Url: origin.RootIdentity(), CreditBalance: 1e9})
	simul.CreateAccount(&protocol.LiteTokenAccount{Url: origin, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(1e9)})
	return nil
}

func CreateAccumulateSnapshot() {
	dir, _ := os.UserHomeDir()
	filename := func(partition string) string {
		return filepath.Join(dir, fmt.Sprintf("%s.bpt", partition))
	}
	for _, partition := range simul.Partitions {
		x := simul.Partition(partition.Id)
		batch := x.Database.Begin(false)
		defer batch.Discard()
		f, err := os.Create(filename(partition.Id))
		if err != nil {
			log.Panicln(err.Error())
		}
		x.Executor.SaveSnapshot(batch, f)
	}
}

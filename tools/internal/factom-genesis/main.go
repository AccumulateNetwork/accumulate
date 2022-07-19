package factom

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"time"

	f2 "github.com/FactomProject/factom"
	tmjson "github.com/tendermint/tendermint/libs/json"
	"github.com/tendermint/tendermint/privval"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/cmd"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

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
	url, err := protocol.LiteTokenAddress(key.PublicKey, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		log.Fatalf("cannot create lite token account %v", err)
	}
	log.Println("URL : ", url)

	origin = url
	return nil
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
		log.Println("TxId : ", res.TxID)
		request := query.RequestByTxId{
			TxId: res.TxID.Hash(),
		}
		txRes := simul.Query(res.TxID.Account(), &request, true)
		log.Println("Txn Query Res : ", txRes)
	}
	log.Println("Wrote to : ", dataAccount.String())
	return nil
}

func ExecuteDataEntry(chainId *[32]byte, entry *protocol.FactomDataEntry) {
	u, err := protocol.LiteDataAddress((*chainId)[:])
	if err != nil {
		log.Fatalf("error creating lite address %x, %v", *chainId, err)
	}
	WriteDataToSim(u, entry)
}

func WriteDataToSim(chainUrl *url.URL, entry *protocol.FactomDataEntry) {
	err := WriteDataToAccumulateSim(entry, chainUrl)
	if err != nil {
		log.Printf("error writing data to accumulate : %v", err)
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
		if err := x.Executor.SaveSnapshot(batch, f); err != nil {
			log.Println("Snapshot error : ", err.Error())
		}
	}
}

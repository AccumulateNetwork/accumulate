package factom

import (
	"crypto/ed25519"
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
	alog "github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/privval"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

var origin *url.URL
var key *walletd.Key
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
	opts := simulator.SimulatorOptions{
		BvnCount: 3,
		OpenDB:   openDB,
	}
	sim := simulator.NewWith(simTb{}, opts)
	simul = sim
	simul.InitFromGenesis()
}

func openDB(partition string, nodeIndex int, logger alog.Logger) *database.Database {
	dir := os.TempDir() + "/tempbadger"
	db, err := database.OpenBadger(dir+partition, nil)
	if err != nil {
		log.Fatal(err)
	}
	return db
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

	key = &walletd.Key{PrivateKey: priv, PublicKey: pub, KeyInfo: walletd.KeyInfo{Type: protocol.SignatureTypeED25519}}
	url, err := protocol.LiteTokenAddress(key.PublicKey, protocol.ACME, protocol.SignatureTypeED25519)
	if err != nil {
		log.Fatalf("cannot create lite token account %v", err)
	}
	log.Println("URL : ", url)

	origin = url
	return nil
}

func ConstructWriteDataToSim(data protocol.DataEntry, dataAccount *url.URL) *protocol.Transaction {
	// log.Println("Writing to : ", dataAccount.String())
	wd := &protocol.WriteDataTo{
		Entry:     &protocol.AccumulateDataEntry{Data: data.GetData()},
		Recipient: dataAccount,
	}
	txn := new(protocol.Transaction)
	txn.Header.Principal = origin
	txn.Body = wd
	return txn

	// log.Println("Executing txn")
	// responses, _ := simul.WaitForTransactions(delivered, simul.MustSubmitAndExecuteBlock(
	// 	acctesting.NewTransaction().
	// 		WithPrincipal(origin).
	// 		WithTimestampVar(&timestamp).
	// 		WithSigner(origin, 1).
	// 		WithBody(wd).
	// 		Initiate(protocol.SignatureTypeED25519, key.PrivateKey).
	// 		Build())...)
	// for _, res := range responses {
	// 	log.Println("TxId : ", res.TxID)
	// 	request := query.RequestByTxId{
	// 		TxId: res.TxID.Hash(),
	// 	}
	// 	txRes := simul.Query(res.TxID.Account(), &request, true)
	// 	log.Println("Txn Query Res : ", txRes)
	// }
	// log.Println("Wrote to : ", dataAccount.String())
}

func ConstructWriteData(u *url.URL, entry *protocol.FactomDataEntry) *protocol.Transaction {
	return ConstructWriteDataToSim(entry, u)
}

func GetAccountFromPrivateString(hexString string) *url.URL {
	var key walletd.Key
	privKey, err := hex.DecodeString(hexString)
	if err == nil && len(privKey) == 64 {
		key.PrivateKey = privKey
		key.PublicKey = privKey[32:]
		key.KeyInfo.Type = protocol.SignatureTypeED25519
	}
	return protocol.LiteAuthorityForKey(key.PublicKey, key.KeyInfo.Type)
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
	// Generate a key
	h := storage.MakeKey("factom-genesis")
	liteKey := ed25519.NewKeyFromSeed(h[:])
	key = &walletd.Key{PublicKey: liteKey[32:], PrivateKey: liteKey, KeyInfo: walletd.KeyInfo{Type: protocol.SignatureTypeED25519}}
	lite, err := protocol.LiteTokenAddress(key.PublicKey, protocol.ACME, key.KeyInfo.Type)
	if err != nil {
		return err
	}
	origin = lite.RootIdentity()

	simul.WaitForTransactions(delivered, simul.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(protocol.FaucetUrl).
			WithBody(&protocol.AcmeFaucet{Url: lite}).
			Faucet(),
	)...)

	simul.WaitForTransactions(delivered, simul.MustSubmitAndExecuteBlock(
		acctesting.NewTransaction().
			WithPrincipal(lite).
			WithSigner(lite, 1).
			WithBody(&protocol.AddCredits{
				Recipient: lite.RootIdentity(),
				Oracle:    protocol.InitialAcmeOracleValue,
				Amount:    *big.NewInt(protocol.AcmeFaucetAmount * protocol.AcmePrecision),
			}).
			WithTimestamp(1).
			Initiate(protocol.SignatureTypeED25519, key.PrivateKey).
			Build(),
	)...)

	return nil
}

func CreateAccumulateSnapshot() {
	dir, _ := os.UserHomeDir()
	filename := func(partition string) string {
		return filepath.Join(dir, fmt.Sprintf("%s.bpt", partition))
	}
	t := time.Now()
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
	fmt.Printf("Saved %d snapshots in %v\n", len(simul.Partitions), time.Since(t)) //nolint:noprint
}

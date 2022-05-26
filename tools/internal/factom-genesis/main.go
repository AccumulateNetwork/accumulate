package factom

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/cmd"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var factomChainData map[[32]byte]*Queue
var chainQueue map[string]bool
var origin *url.URL
var key *cmd.Key

const (
	LOCAL_URL = "http://127.0.1.1:26660"
)

func AccountFromPrivateKey(privateKey, publicKey string) (*url.URL, error) {
	var pk ed25519.PrivateKey
	if len([]byte(privateKey)) == 32 {
		pk = ed25519.NewKeyFromSeed([]byte(privateKey))
	} else {
		pk = []byte(privateKey)
	}

	url, _ := protocol.LiteTokenAddress(pk[32:], protocol.ACME, protocol.SignatureTypeED25519)
	key = &cmd.Key{PrivateKey: []byte(publicKey), PublicKey: []byte(privateKey), Type: protocol.SignatureTypeED25519}
	origin = url.RootIdentity()
	return url, nil
}

func buildEnvelope(payload protocol.TransactionBody) (*protocol.Envelope, error) {
	txn := new(protocol.Transaction)
	txn.Body = payload
	txn.Header.Principal = origin
	signer := new(signing.Builder)
	signer.SetPrivateKey(key.PrivateKey)
	signer.SetTimestampToNow()
	signer.SetVersion(1)
	signer.SetType(protocol.SignatureTypeED25519)
	signer.SetUrl(origin)

	sig, err := signer.Initiate(txn)
	if err != nil {
		fmt.Println("Error : ", err.Error())
		return nil, err
	}

	envelope := new(protocol.Envelope)
	envelope.Transaction = append(envelope.Transaction, txn)
	envelope.Signatures = append(envelope.Signatures, sig)
	envelope.TxHash = append(envelope.TxHash, txn.GetHash()...)

	return envelope, nil
}

func WriteDataToAccumulate(env string, data protocol.DataEntry, dataAccount *url.URL) error {
	client, err := client.New(env)

	if err != nil {
		fmt.Println("Error : ", err.Error())
		return err
	}

	wd := &protocol.WriteDataTo{
		Entry:     data,
		Recipient: dataAccount,
	}

	envelope, err := buildEnvelope(wd)
	if err != nil {
		return err
	}

	req := new(api.ExecuteRequest)
	req.Envelope = envelope

	res, err := client.ExecuteDirect(context.Background(), req)
	if err != nil {
		fmt.Println("Error : ", err.Error())
		return err
	}
	if res.Code != 0 {
		fmt.Println(res.Message)
		return fmt.Errorf(res.Message)
	}
	return nil
}

func WriteDataFromQueueToAccumulate() {
	for chainId, data := range factomChainData {
		// go ExecuteQueueToWriteData(chainId, data)

		chainUrl, err := protocol.LiteDataAddress(chainId[:])
		if err != nil {
			fmt.Println("Error : ", err.Error())
			break
		}
		ExecuteQueueToWriteData(chainUrl, data)
	}
}

func ExecuteQueueToWriteData(chainUrl *url.URL, queue *Queue) error {
	for {
		if len(*queue) > 0 {
			entry := queue.Pop().(*Entry)
			dataEntry := ConvertFactomDataEntryToLiteDataEntry(*entry)
			WriteDataToAccumulate(LOCAL_URL, &protocol.AccumulateDataEntry{Data: dataEntry.GetData()}, chainUrl)
		} else {
			break
		}
	}
	return nil
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

func ConvertFactomDataEntryToLiteDataEntry(entry Entry) *protocol.FactomDataEntry {
	dataEntry := new(protocol.FactomDataEntry)
	copy(dataEntry.AccountId[:], entry.ChainId)
	dataEntry.Data = []byte(entry.Content)
	dataEntry.ExtIds = entry.ExtIds
	return dataEntry
}

func GetDataAndPopulateQueue(entries []*Entry) {
	factomChainData = make(map[[32]byte]*Queue)
	for _, entry := range entries {
		accountId := *(*[32]byte)(entry.ChainId)
		_, ok := factomChainData[accountId]
		if !ok {
			factomChainData[accountId] = NewQueue()
		}
		factomChainData[accountId].Push(entry)
	}
}

func nonceFromTimeNow() uint64 {
	t := time.Now()
	return uint64(t.Unix()*1e6) + uint64(t.Nanosecond())/1e3
}

//FaucetWithCredits is only used for testing. Initial account will be prefunded.
func FaucetWithCredits(env string) error {
	client, err := client.New(env)
	if err != nil {
		return err
	}
	faucet := protocol.AcmeFaucet{}
	faucet.Url = origin
	resp, err := client.Faucet(context.Background(), &faucet)
	if err != nil {
		return err
	}

	txReq := api.TxnQuery{}
	txReq.Txid = resp.TransactionHash
	txReq.Wait = time.Second * 10
	txReq.IgnorePending = false

	_, err = client.QueryTx(context.Background(), &txReq)
	if err != nil {
		return err
	}

	//now buy a bunch of credits.
	cred := protocol.AddCredits{}
	cred.Recipient = origin
	cred.Oracle = 500
	cred.Amount.SetInt64(100000000000000)

	envelope, err := buildEnvelope(&cred)
	if err != nil {
		return err
	}

	_, err = client.ExecuteDirect(context.Background(), &api.ExecuteRequest{Envelope: envelope})
	if err != nil {
		return err
	}
	return nil
}

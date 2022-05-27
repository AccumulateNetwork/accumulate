package factom

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	f2 "github.com/FactomProject/factom"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"

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

func AccountFromPrivateKey(privateKey []byte) (*url.URL, error) {
	var pk ed25519.PrivateKey
	if len(privateKey) == 32 || len(privateKey) == 64 {
		pk = ed25519.NewKeyFromSeed(privateKey)
	} else {
		return nil, fmt.Errorf("invalid private key, cannot create account")
	}

	url, _ := protocol.LiteTokenAddress(pk[32:], protocol.ACME, protocol.SignatureTypeED25519)
	key = &cmd.Key{PrivateKey: pk, PublicKey: pk[32:], Type: protocol.SignatureTypeED25519}
	origin = url
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
		fmt.Println("Response Error : ", res.Message)
		return fmt.Errorf(res.Message)
	}
	//TODO: Read back data to confirm it wrote, or write a separate function to verify data
	//TODO: formulate factom entry hash from data, also consider passing in the orig. entry hash obtained from factom -> compare those hashes
	//TODO: query entry hash
	//TODO: here is the get call. if the get works then it should be ok.

	// queryUrl, err := url.Parse(dataAccount.String() + "#data/" + hex.EncodeToString(data.Hash()))
	// if err != nil {
	// 	log.Println("URL error : ", err.Error())
	// 	return err
	// }
	// genQuery := &api.GeneralQuery{}
	// genQuery.Url = queryUrl
	// resp, err := client.Query(context.Background(), genQuery)
	// if err != nil {
	// 	log.Println("Error : ", err.Error())
	// 	return err
	// }
	// log.Println("Response : ", resp)

	txReq := api.TxnQuery{}
	txReq.Txid = res.TransactionHash
	txReq.Wait = time.Second * 10
	txReq.IgnorePending = false

	_, err = client.QueryTx(context.Background(), &txReq)
	if err != nil {
		return err
	}

	var dataHash [32]byte
	copy(dataHash[:], data.Hash())
	queryReq := &api.DataEntryQuery{
		Url:       dataAccount,
		EntryHash: dataHash,
	}
	queryRes, err := client.QueryData(context.Background(), queryReq)
	if err != nil {
		log.Println("Error : ", err.Error())
		return err
	}
	log.Println("Response : ", queryRes.Data)
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

		log.Printf("Writing data to %s", chainUrl.String())
		ExecuteQueueToWriteData(chainUrl, data)
	}
}

func ExecuteQueueToWriteData(chainUrl *url.URL, queue *Queue) error {

	for {
		if len(*queue) > 0 {
			entry := queue.Pop().(*f2.Entry)
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

func ConvertFactomDataEntryToLiteDataEntry(entry f2.Entry) *protocol.FactomDataEntry {
	dataEntry := new(protocol.FactomDataEntry)
	copy(dataEntry.AccountId[:], entry.ChainID)
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
	cred.Amount.SetInt64(200000000000000)

	envelope, err := buildEnvelope(&cred)
	if err != nil {
		return err
	}

	resp, err = client.ExecuteDirect(context.Background(), &api.ExecuteRequest{Envelope: envelope})
	if err != nil {
		return err
	}

	txReq = api.TxnQuery{}
	txReq.Txid = resp.TransactionHash
	txReq.Wait = time.Second * 10
	txReq.IgnorePending = false

	_, err = client.QueryTx(context.Background(), &txReq)
	if err != nil {
		return err
	}

	return nil
}

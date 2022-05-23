package factom

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/cmd"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var factomChainData map[string]*Queue
var chainQueue map[string]bool
var origin *url.URL
var signer *signing.Builder
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

	key = &cmd.Key{PrivateKey: []byte(publicKey), PublicKey: []byte(privateKey), Type: protocol.SignatureTypeED25519}
	signer = new(signing.Builder)
	signer.Type = protocol.SignatureTypeLegacyED25519
	signer.Timestamp = nonceFromTimeNow()
	signer.Type = key.Type
	signer.Version = 1
	signer.SetPrivateKey(key.PrivateKey)
	url, _ := protocol.LiteTokenAddress(pk[32:], protocol.ACME, protocol.SignatureTypeED25519)
	origin = url
	signer.Url = url.RootIdentity()
	fmt.Println(signer.Url)
	return url, nil
}

func WriteDataToAccumulate(env string, data *protocol.LiteDataEntry) error {
	client, err := client.New(env)
	if err != nil {
		fmt.Println("Error : ", err.Error())
		return err
	}
	wd := &protocol.WriteDataTo{
		Entry: *data.DataEntry,
	}
	txn := new(protocol.Transaction)
	txn.Body = wd
	txn.Header.Principal = origin
	sig, err := signer.Initiate(txn)
	if err != nil {
		fmt.Println("Error : ", err.Error())
		return err
	}
	addressString, err := protocol.GetFactoidAddressFromRCDHash(data.AccountId[:])
	if err != nil {
		fmt.Println("Error : ", err.Error())
		return err
	}
	chainUrl, err := protocol.GetLiteAccountFromFactoidAddress(addressString)
	if err != nil {
		fmt.Println("Error : ", err.Error())
		return err
	}
	wd.Recipient = chainUrl
	req := &api.TxRequest{
		Payload:   wd,
		Origin:    origin,
		Signature: sig.GetSignature(),
		Signer: api.Signer{
			Timestamp: nonceFromTimeNow(),
			PublicKey: key.PublicKey,
			Url:       origin,
			Version:   1,
		},
	}
	res, err := client.ExecuteWriteDataTo(context.Background(), req)
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
		ExecuteQueueToWriteData(chainId, data)
	}
}

func ExecuteQueueToWriteData(chainId string, queue *Queue) {
	for {
		if len(*queue) > 0 {
			entry := queue.Pop().(*Entry)
			dataEntry := ConvertFactomDataEntryToLiteDataEntry(*entry)
			WriteDataToAccumulate(LOCAL_URL, dataEntry)
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

func ConvertFactomDataEntryToLiteDataEntry(entry Entry) *protocol.LiteDataEntry {
	dataEntry := protocol.LiteDataEntry{
		DataEntry: &protocol.DataEntry{},
	}
	copy(dataEntry.AccountId[:], entry.ChainId)
	dataEntry.DataEntry.Data = append(dataEntry.Data, []byte(entry.Content))
	dataEntry.DataEntry.Data = append(dataEntry.Data, entry.ExtIds...)
	return &dataEntry
}

func GetDataAndPopulateQueue(entries []*Entry) {
	factomChainData = make(map[string]*Queue)
	for _, entry := range entries {
		_, ok := factomChainData[string(entry.ChainId)]
		if !ok {
			factomChainData[string(entry.ChainId)] = NewQueue()
		}
		factomChainData[string(entry.ChainId)].Push(entry)
	}
}

func nonceFromTimeNow() uint64 {
	t := time.Now()
	return uint64(t.Unix()*1e6) + uint64(t.Nanosecond())/1e3
}

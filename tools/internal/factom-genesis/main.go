package factom

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tendermint/tendermint/privval"
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

const (
	LOCAL_URL = "http://localhost:26660"
)

func AccountFromPrivateKey(privateKeyHex string) (*url.URL, error) {
	privBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, err
	}
	var pvkey privval.FilePVKey
	err = json.Unmarshal(privBytes, &pvkey)
	if err != nil {
		return nil, err
	}
	var pub, priv []byte
	if pvkey.PubKey != nil {
		pub = pvkey.PubKey.Bytes()
	}
	if pvkey.PrivKey != nil {
		priv = pvkey.PrivKey.Bytes()
	}
	key := &cmd.Key{PrivateKey: pub, PublicKey: priv, Type: protocol.SignatureTypeED25519}
	signer = new(signing.Builder)
	signer.Type = protocol.SignatureTypeLegacyED25519
	signer.Timestamp = nonceFromTimeNow()
	signer.Type = key.Type
	signer.Version = 1
	signer.SetPrivateKey(key.PrivateKey)
	url := protocol.LiteAuthorityForKey(key.PublicKey, protocol.SignatureTypeED25519)
	origin = url
	signer.Url = url.RootIdentity()
	fmt.Println(signer.Url)
	return url, nil
}

func WriteDataToAccumulate(env string, data *protocol.LiteDataEntry) error {
	client, err := client.New(env)
	if err != nil {
		return err
	}
	wd := &protocol.WriteData{
		Entry: *data.DataEntry,
	}
	txn := new(protocol.Transaction)
	txn.Body = wd
	txn.Header.Principal = origin
	sig, err := signer.Initiate(txn)
	if err != nil {
		return err
	}
	req := &api.TxRequest{
		Payload:   wd,
		Origin:    origin,
		Signature: sig.GetSignature(),
	}
	_, err = client.ExecuteWriteDataTo(context.Background(), req)
	if err != nil {
		return err
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
	if len(*queue) > 0 {
		entry := queue.Pop().(*Entry)
		dataEntry := ConvertFactomDataEntryToLiteDataEntry(*entry)
		WriteDataToAccumulate(LOCAL_URL, dataEntry)
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
	fmt.Println(string(entry.ChainId))
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

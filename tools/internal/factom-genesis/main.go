package factom

import (
	"context"
	"encoding/hex"

	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/cmd"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var factomChainData map[string]*Queue

func accountFromPrivateKey(privateKeyHex string) {

}

func WriteDataToAccumulate(env string, data *protocol.LiteDataEntry) error {
	client, err := client.New(env)
	if err != nil {
		return err
	}
	req := &api.TxRequest{}
	_, err = client.ExecuteWriteDataTo(context.Background(), req)
	if err != nil {
		return err
	}
	return nil
}

func WriteDataFromQueueToAccumulate() {
	for chainId, data := range factomChainData {
		go executeQueueToWriteData(chainId, data)
	}
}

func executeQueueToWriteData(chainId string, queue *Queue) {
	if len(*queue) > 0 {
		data := queue.Pop().(*protocol.LiteDataEntry)
		WriteDataToAccumulate("", data)
	}
}

func getAccountFromPrivateString(hexString string) *url.URL {
	var key cmd.Key
	privKey, err := hex.DecodeString(hexString)
	if err == nil && len(privKey) == 64 {
		key.PrivateKey = privKey
		key.PublicKey = privKey[32:]
		key.Type = protocol.SignatureTypeED25519
	}
	return protocol.LiteAuthorityForKey(key.PublicKey, key.Type)
}

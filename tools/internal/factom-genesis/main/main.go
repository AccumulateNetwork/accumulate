package main

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/client"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/factom-genesis"
)

func main() {
	url, _ := factom.AccountFromPrivateKey("1bee1f90330115a13d2f3fe2dac2208b2a1118482b6e3b4375e567e5d84c9bf13018938f9ba9b66af0ceb0d33d5632a8b001fddd3e1eeaa9219587e5aa178448")
	fmt.Println("URL : ", url)
	entries := factom.CurlEntryFromFactom()
	factom.GetDataAndPopulateQueue(entries)
	factom.WriteDataFromQueueToAccumulate()
}

func GetFaucet(env string) error {
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
	_, err = client.Faucet(context.Background(), req)
	if err != nil {
		return err
	}
	return nil
}

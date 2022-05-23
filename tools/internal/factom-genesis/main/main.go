package main

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/tools/internal/factom-genesis"
)

const (
	Key_Name        = "keytest-0-0"
	Key_Private_Key = "d125672c7f0af6fd82c87c884560c6fcbaf03bcd51ac578057369d7e99274f3c"
	Key_Public_Key  = "f40e4b1b3bf80938c4e9a541f395af8fd8a3ef39ea779b3bd85ce65fb17deb72"
)

func main() {
	url, _ := factom.AccountFromPrivateKey(Key_Private_Key, Key_Public_Key)
	fmt.Println("URL : ", url)
	entries := factom.CurlEntryFromFactom()
	factom.GetDataAndPopulateQueue(entries)
	factom.WriteDataFromQueueToAccumulate()
}

// func GetFaucet(env string) error {
// 	client, err := client.New(env)
// 	if err != nil {
// 		return err
// 	}
// 	wd := &protocol.WriteData{
// 		Entry: *data.DataEntry,
// 	}
// 	txn := new(protocol.Transaction)
// 	txn.Body = wd
// 	txn.Header.Principal = origin
// 	sig, err := signer.Initiate(txn)
// 	if err != nil {
// 		return err
// 	}
// 	req := &api.TxRequest{
// 		Payload:   wd,
// 		Origin:    origin,
// 		Signature: sig.GetSignature(),
// 	}
// 	_, err = client.Faucet(context.Background(), req)
// 	if err != nil {
// 		return err
// 	}
// 	return nil
// }

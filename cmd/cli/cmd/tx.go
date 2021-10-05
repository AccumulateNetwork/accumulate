package cmd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/AccumulateNetwork/accumulated/types"
	acmeapi "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/boltdb/bolt"
	"github.com/spf13/cobra"
)

var txCmd = &cobra.Command{
	Use:   "tx",
	Short: "Create and get token txs",
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "get":
				if len(args) > 2 {
					GetTX(args[1], args[2])
				} else {
					fmt.Println("Usage:")
					PrintTXGet()
				}
			case "create":
				if len(args) > 3 {
					CreateTX(args[1], args[2], args[3])
				} else {
					fmt.Println("Usage:")
					PrintTXCreate()
				}
			default:
				fmt.Println("Usage:")
				PrintTX()
			}
		} else {
			fmt.Println("Usage:")
			PrintTX()
		}

	},
}

func init() {
	rootCmd.AddCommand(txCmd)
}

func PrintTXGet() {
	fmt.Println("  accumulate tx get [token account] [txid]			Get token tx by token account and txid")
}

func PrintTXCreate() {
	fmt.Println("  accumulate tx create [from] [to] [amount]	Create new token tx")
}

func PrintTX() {
	PrintTXGet()
	PrintTXCreate()
}

func GetTX(account string, hash string) {

	var res interface{}
	var str []byte
	var hashbytes types.Bytes32

	params := new(acmeapi.TokenTxRequest)
	params.From = types.UrlChain{types.String(account)}
	err := hashbytes.FromString(hash)
	if err != nil {
		log.Fatal(err)
	}

	params.Hash = hashbytes

	data, err := json.Marshal(params)
	jsondata := json.RawMessage(data)
	if err != nil {
		log.Fatal(err)
	}

	if err := Client.Request(context.Background(), "token-tx", jsondata, &res); err != nil {
		log.Fatal(err)
	}

	str, err = json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))

}

func CreateTX(sender string, receiver string, amount string) {

	var res interface{}
	var str []byte
	var err error

	err = Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("anon"))
		pk := b.Get([]byte(sender))
		fmt.Println(hex.EncodeToString(pk))
		params := &acmeapi.APIRequestRaw{}
		params.Tx = &acmeapi.APIRequestRawTx{}

		tokentx := new(acmeapi.TokenTx)
		tokentx.From = types.UrlChain{types.String(sender)}

		to := []*acmeapi.TokenTxOutput{}
		r := &acmeapi.TokenTxOutput{}
		r.Amount, err = strconv.ParseUint(amount, 10, 64)
		r.URL = types.UrlChain{types.String(receiver)}
		to = append(to, r)

		data, err := json.Marshal(tokentx)
		if err != nil {
			log.Fatal(err)
		}

		datajson := json.RawMessage(data)
		params.Tx.Data = &datajson
		params.Tx.Timestamp = time.Now().Unix()
		params.Tx.Signer = &acmeapi.Signer{}
		params.Tx.Signer.URL = types.String(sender)
		params.Tx.Signer.PublicKey = types.Bytes32{}

		params.Sig = types.Bytes64{}

		if err := Client.Request(context.Background(), "token-tx-create", params, &res); err != nil {
			log.Fatal(err)
		}

		str, err = json.Marshal(res)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(string(str))
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

}

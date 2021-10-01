package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/AccumulateNetwork/accumulated/types"
	acmeapi "github.com/AccumulateNetwork/accumulated/types/api"
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
				if len(args) > 4 {
					CreateTX(args[1], args[2], args[3], args[4])
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

	params := acmeapi.TokenTx{}
	params.From = types.UrlChain{types.String(account)}
	hashbytes.FromString(hash)
	params.Hash = hashbytes

	jsondata, err := json.Marshal(params)
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

func CreateTX(sender string, receiver string, amount string, signer string) {
	fmt.Println("Creating new token tx")
}

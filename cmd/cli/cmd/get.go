package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/AccumulateNetwork/accumulate/types"
	acmeapi "github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/spf13/cobra"
)

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Get data by URL",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if len(args) > 0 {
			switch args[0] {
			case "chain":
				if len(args) > 1 {
					GetByChainId([]byte(args[1]))
				} else {
					fmt.Println("Usage:")
					PrintGet()
				}
			default:
				if len(args) > 0 {
					out, err = Get(args[0])
				} else {
					fmt.Println("Usage:")
					PrintGet()
				}
			}
		} else {
			fmt.Println("Usage:")
			PrintGet()
		}
		if err != nil {
			cmd.Print("Error: ")
			cmd.PrintErr(err)
		} else {
			cmd.Println(out)
		}
	},
}

func PrintGet() {
	fmt.Println("  accumulate get [url] 		Get data by Accumulate URL")
	//fmt.Println("  accumulate get [chain id] 		Get data by Accumulate chain id")
	//fmt.Println("  accumulate get [transaction id] 		Get data by Accumulate transaction id")
}

func GetByChainId(chainId []byte) (*acmeapi.APIDataResponse, error) {
	var res interface{}
	//var str []byte

	params := acmeapi.APIRequestChainId{}
	params.ChainId = chainId

	str1, err1 := json.Marshal(&params)
	if err1 != nil {
		return nil, err1
	}

	fmt.Println(string(str1))

	if err := Client.Request(context.Background(), "chain", params, &res); err != nil {
		log.Fatal(err)
	}

	resp := res.(acmeapi.APIDataResponse)

	return &resp, nil
}

func Get(url string) (string, error) {
	var res interface{}

	params := acmeapi.APIRequestURL{}
	params.URL = types.String(url)

	if err := Client.Request(context.Background(), "get", params, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	str, err := json.Marshal(res)
	if err != nil {
		return "", err
	}

	return string(str), nil
}

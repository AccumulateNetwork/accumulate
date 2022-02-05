package cmd

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	api2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/types"
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
					chainId := types.Bytes32{}
					err = chainId.FromString(args[1])
					if err == nil {
						out, err = GetByChainId(chainId[:])
					}
				} else {
					fmt.Println("Usage:")
					PrintGet()
				}
			case "key":
				if len(args) > 2 {
					out, err = GetKey(args[1], args[2])
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
		printOutput(cmd, out, err)
	},
}

func PrintGet() {
	fmt.Println("  accumulate get [url] 		Get data by Accumulate URL")
	//fmt.Println("  accumulate get [chain id] 		Get data by Accumulate chain id")
	//fmt.Println("  accumulate get [transaction id] 		Get data by Accumulate transaction id")
}

func GetByChainId(chainId []byte) (string, error) {
	params := api2.ChainIdQuery{}
	params.ChainId = chainId

	res, err := Client.QueryChain(context.Background(), &params)
	return PrintAccountQueryResponse(res, err)
}

func Get(url string) (string, error) {
	params := api2.GeneralQuery{}
	params.Url = url

	res, err := Client.Query(context.Background(), &params, nil)
	if err != nil {
		return "", formatApiError(err)
	}

	if WantJsonOutput {
		return PrintJson(res)
	}

	switch res := res.(type) {
	case *api.ChainQueryResponse:
		return PrintAccountQueryResponse(res, nil)
	case *api.TransactionQueryResponse:
		return PrintTransactionQueryResponseV2(res)
	case *api.MultiResponse:
		return PrintMultiResponse(res, nil)
	default:
		return PrintJson(res)
	}
}

func GetKey(url, key string) (string, error) {
	keyb, err := hex.DecodeString(key)
	if err != nil {
		return "", err
	}

	res, err := getKey(url, keyb)
	if err != nil {
		return "", err
	}

	if WantJsonOutput {
		return PrintJson(res)
	}

	var out string
	out += fmt.Sprintf("Key book\t:\t%v\n", res.KeyBook)
	out += fmt.Sprintf("Key page\t:\t%v (index=%v)\n", res.KeyPage, res.Index)
	return out, nil
}

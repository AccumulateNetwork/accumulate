package cmd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"

	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/query"
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
					chainId := types.Bytes32{}
					err = chainId.FromString(args[1])
					if err == nil {
						var q *api2.ChainQueryResponse
						q, err = GetByChainId(chainId[:])
						if err == nil {
							var data []byte
							data, err = json.Marshal(q)
							if err == nil {
								out = string(data)
							}
						}
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

var GetDirect bool

func init() {
	getCmd.Flags().BoolVar(&GetDirect, "direct", false, "Use debug-query-direct instead of query")
}

func PrintGet() {
	fmt.Println("  accumulate get [url] 		Get data by Accumulate URL")
	//fmt.Println("  accumulate get [chain id] 		Get data by Accumulate chain id")
	//fmt.Println("  accumulate get [transaction id] 		Get data by Accumulate transaction id")
}

func GetByChainId(chainId []byte) (*api2.ChainQueryResponse, error) {
	var res api2.ChainQueryResponse

	params := api2.ChainIdQuery{}
	params.ChainId = chainId

	data, err := json.Marshal(&params)
	if err != nil {
		return nil, err
	}

	if err := Client.Request(context.Background(), "query-chain", json.RawMessage(data), &res); err != nil {
		log.Fatal(err)
	}

	return &res, nil
}

func Get(url string) (string, error) {
	params := api2.UrlQuery{}
	params.Url = url

	data, err := json.Marshal(&params)
	if err != nil {
		return "", err
	}

	method := "query"
	if GetDirect {
		method = "debug-query-direct"
	}

	var res json.RawMessage
	err = Client.Request(context.Background(), method, json.RawMessage(data), &res)
	if err == nil {
		return string(res), nil
	}

	ret, err := PrintJsonRpcError(err)
	if err != nil {
		return "", err
	}

	return "", fmt.Errorf("%v", ret)
}

func GetKey(url, key string) (string, error) {
	var res api2.ChainQueryResponse
	res.Data = new(query.ResponseKeyPageIndex)

	keyb, err := hex.DecodeString(key)
	if err != nil {
		return "", err
	}

	params := new(api2.KeyPageIndexQuery)
	params.Url = url
	params.Key = keyb

	err = Client.Request(context.Background(), "query-key-index", &params, &res)
	if err != nil {
		return PrintJsonRpcError(err)
	}

	str, err := json.Marshal(res)
	if err != nil {
		return "", err
	}

	return string(str), nil
}

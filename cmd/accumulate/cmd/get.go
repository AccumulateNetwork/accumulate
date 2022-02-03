package cmd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	api2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
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

	if err := Client.RequestAPIv2(context.Background(), "query-chain", json.RawMessage(data), &res); err != nil {
		log.Fatal(err)
	}

	return &res, nil
}

func Get(url string) (string, error) {
	params := api2.UrlQuery{}
	params.Url = url

	method := "query"
	if GetDirect {
		method = "debug-query-direct"
	}

	var res json.RawMessage
	err := queryAs(method, &params, &res)
	if err != nil {
		return "", err
	}

	if WantJsonOutput {
		return string(res), nil
	}

	// Is it an account?
	if json.Unmarshal(res, new(struct{ Type types.AccountType })) == nil {
		qr := new(QueryResponse)
		if json.Unmarshal(res, qr) != nil {
			return string(res), nil
		}
		return PrintChainQueryResponseV2(qr)
	}

	// Is it a transaction?
	if json.Unmarshal(res, new(struct{ Type types.TransactionType })) == nil {
		qr := new(api2.TransactionQueryResponse)
		if json.Unmarshal(res, qr) != nil {
			return string(res), nil
		}
		return PrintTransactionQueryResponseV2(qr)
	}

	return string(res), nil
}

func getKey(url string, key []byte) (*query.ResponseKeyPageIndex, error) {
	params := new(api2.KeyPageIndexQuery)
	params.Url = url
	params.Key = key

	res := new(query.ResponseKeyPageIndex)
	qres := new(api2.ChainQueryResponse)
	qres.Data = res

	err := queryAs("query-key-index", &params, &qres)
	if err != nil {
		return nil, err
	}

	return res, nil
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
		str, err := json.Marshal(res)
		if err != nil {
			return "", err
		}

		return string(str), nil
	}

	var out string
	out += fmt.Sprintf("Key book\t:\t%v\n", res.KeyBook)
	out += fmt.Sprintf("Key page\t:\t%v (index=%v)\n", res.KeyPage, res.Index)
	return out, nil
}

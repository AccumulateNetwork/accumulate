package cmd

import (
	"fmt"
	"strconv"

	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/spf13/cobra"
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Issue and get tokens",
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "get":
				if len(args) > 1 {
					GetToken(args[1])
				} else {
					fmt.Println("Usage:")
					PrintTokenGet()
				}
			case "create":
				if len(args) > 4 {
					var propertiesUrl string
					if len(args) > 5 {
						propertiesUrl = args[6]
					}
					CreateToken(args[1], args[2], args[3], args[4], args[5], propertiesUrl)
				} else {
					fmt.Println("Usage:")
					PrintTokenCreate()
				}
			default:
				fmt.Println("Usage:")
				PrintToken()
			}
		} else {
			fmt.Println("Usage:")
			PrintToken()
		}

	},
}

func PrintTokenGet() {
	fmt.Println("  accumulate token get [url] Get token by URL")
}

func PrintTokenCreate() {
	fmt.Println("  accumulate token create [origin adi url] [signer key name] [url] [symbol] [precision (0 - 18)] [properties URL (optional)] 	Create new token")
}

func PrintToken() {
	PrintTokenGet()
	PrintTokenCreate()
}

func GetToken(url string) (*QueryResponse, *protocol.TokenAccount, error) {

	res, err := GetUrl(url)
	if err != nil {
		return nil, nil, err
	}

	tokenAccount := protocol.TokenAccount{}
	err = Remarshal(res.Data, &tokenAccount)
	if err != nil {
		return nil, nil, err
	}
	return res, &tokenAccount, nil

}

func CreateToken(origin, signer, url, symbol, precision, properties string) (string, error) {
	u, err := url2.Parse(url)
	if err != nil {
		return "", err
	}

	originUrl, err := url2.Parse(origin)
	if err != nil {
		return "", err
	}

	_, si, privKey, err := prepareSigner(u, []string{signer})
	if err != nil {
		return "", err
	}

	prcsn, err := strconv.Atoi(precision)
	if err != nil {
		return "", err
	}

	params := protocol.CreateToken{}
	params.Symbol = symbol
	params.Precision = uint64(prcsn)
	params.Properties = properties

	res, err := dispatchTxRequest("create-token", &params, nil, originUrl, si, privKey)
	if err != nil {
		return "", err
	}

	return ActionResponseFrom(res).Print()
}

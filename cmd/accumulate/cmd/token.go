package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Issue and get tokens",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "get":
				if len(args) > 1 {
					out, err = GetToken(args[1])
				} else {
					fmt.Println("Usage:")
					PrintTokenGet()
				}
			case "create":
				if len(args) > 5 {
					var propertiesUrl string
					if len(args) > 6 {
						propertiesUrl = args[6]
					}
					out, err = CreateToken(args[1], args[2], args[3], args[4], args[5], propertiesUrl)
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
		printOutput(cmd, out, err)

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

func GetToken(url string) (string, error) {
	res, err := GetUrl(url)
	if err != nil {
		return "", err
	}

	return PrintChainQueryResponseV2(res)

}

func CreateToken(origin, signer, url, symbol, precision, properties string) (string, error) {
	originUrl, err := url2.Parse(origin)
	if err != nil {
		return "", err
	}

	_, si, privKey, err := prepareSigner(originUrl, []string{signer})
	if err != nil {
		return "", err
	}

	prcsn, err := strconv.Atoi(precision)
	if err != nil {
		return "", err
	}

	u, err := url2.Parse(url)
	if err != nil {
		return "", err
	}

	params := protocol.CreateToken{}
	params.Url = u.String()
	params.Symbol = symbol
	params.Precision = uint64(prcsn)

	if len(properties) > 0 {
		//todo: add GetPropertiesUrl-> as a good measure if propertiesUrl is specified, then we should make sure the url is already populated and valid
		u, err := url2.Parse(properties)
		if err != nil {
			return "", fmt.Errorf("invalid properties url, %v", err)
		}
		params.Properties = u.String()
	}

	params.Properties = properties

	res, err := dispatchTxRequest("create-token", &params, nil, originUrl, si, privKey)
	if err != nil {
		return "", err
	}

	return ActionResponseFrom(res).Print()
}

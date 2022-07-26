package cmd

import (
	"fmt"
	"math"
	"math/big"
	"strconv"

	"github.com/spf13/cobra"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Issue and get tokens",
}

var tokenCmdGet = &cobra.Command{
	Use:   "get [url]",
	Short: "get token by URL",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := GetToken(args[0])
		printOutput(cmd, out, err)
	},
}

var tokenCmdCreate = &cobra.Command{
	Use:   "create [origin adi or lite url] [adi signer key name (if applicable)] [token url] [symbol] [precision (0 - 18)] [supply limit] [properties URL (optional)]",
	Short: "Create new token",
	Args:  cobra.RangeArgs(3, 6),
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if len(args) > 1 {
			out, err = CreateToken(args[0], args[1:])
		} else {
			fmt.Println("Usage:")
			PrintTokenCreate()
		}
		printOutput(cmd, out, err)
	},
}

var tokenCmdIssue = &cobra.Command{
	Use:   "issue [adi token url] [signer key name] [recipient url] [amount]",
	Short: "send tokens from a token url to a recipient",
	Args:  cobra.ExactArgs(4),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := IssueTokenToRecipient(args[0], args[1:])
		printOutput(cmd, out, err)
	},
}

var tokenCmdBurn = &cobra.Command{
	Use:   "burn [adi or lite token account] [adi signer key name (if applicable)] [amount]",
	Short: "burn tokens",
	Args:  cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := BurnTokens(args[0], args[1:])
		printOutput(cmd, out, err)
	},
}

func init() {
	tokenCmd.AddCommand(
		tokenCmdGet,
		tokenCmdCreate,
		tokenCmdIssue,
		tokenCmdBurn)
}

func PrintTokenGet() {
	fmt.Println("  accumulate token get [url] Get token by URL")
}

func PrintTokenCreate() {
	fmt.Println("  accumulate token create [origin adi url] [signer key name] [token url] [symbol] [precision (0 - 18)] [properties URL (optional)] 	Create new token")
	fmt.Println("  accumulate token create [origin lite url] [token url] [symbol] [precision (0 - 18)] [properties URL (optional)] 	Create new token")
}

func GetToken(url string) (string, error) {
	res, err := GetUrl(url)
	if err != nil {
		return "", err
	}

	return PrintChainQueryResponseV2(res)
}

func CreateToken(origin string, args []string) (string, error) {
	originUrl, err := url2.Parse(origin)
	if err != nil {
		return "", err
	}

	args, signer, err := prepareSigner(originUrl, args)
	if err != nil {
		return "", err
	}

	if len(args) < 3 {
		return "", fmt.Errorf("insufficient number of arguments")
	}

	url := args[0]
	symbol := args[1]
	precision := args[2]
	prcsn, err := strconv.Atoi(precision)
	if err != nil {
		return "", err
	}
	var supplyLimit *big.Int
	var properties *url2.URL
	if len(args) > 3 {
		limit, err := strconv.Atoi(args[3])
		if err != nil {
			properties, err = url2.Parse(args[4])
			if err != nil {
				return "", fmt.Errorf("invalid properties url, %v", err)
			}
			res, err := GetUrl(properties.String())
			if err != nil {
				return "", fmt.Errorf("cannot query properties url, %v", err)
			}
			//TODO: make a better test for properties to make sure contents are valid, for now we just see if it is at least a data account
			if res.Type != protocol.AccountTypeDataAccount.String() {
				return "", fmt.Errorf("properties url is not a valid properties data account")
			}
		}
		supplyLimit = big.NewInt(int64(math.Pow10(prcsn)) * int64(limit))
	}
	if len(args) > 4 {
		properties, err = url2.Parse(args[4])
		if err != nil {
			return "", fmt.Errorf("invalid properties url, %v", err)
		}
		res, err := GetUrl(properties.String())
		if err != nil {
			return "", fmt.Errorf("cannot query properties url, %v", err)
		}
		//TODO: make a better test for properties to make sure contents are valid, for now we just see if it is at least a data account
		if res.Type != protocol.AccountTypeDataAccount.String() {
			return "", fmt.Errorf("properties url is not a valid properties data account")
		}
	}

	u, err := url2.Parse(url)
	if err != nil {
		return "", err
	}

	params := protocol.CreateToken{}
	params.Url = u
	params.Symbol = symbol
	params.Precision = uint64(prcsn)
	params.Properties = properties
	params.SupplyLimit = supplyLimit
	for _, authUrlStr := range Authorities {
		authUrl, err := url2.Parse(authUrlStr)
		if err != nil {
			return "", err
		}
		params.Authorities = append(params.Authorities, authUrl)
		params.Authorities = append(params.Authorities, authUrl)
	}

	return dispatchTxAndPrintResponse(&params, originUrl, signer)
}

func IssueTokenToRecipient(origin string, args []string) (string, error) {
	originUrl, err := url2.Parse(origin)
	if err != nil {
		return "", err
	}

	args, signer, err := prepareSigner(originUrl, args)
	if err != nil {
		return "", err
	}

	if len(args) < 2 {
		return "", fmt.Errorf("insufficient number of parameters provided")
	}
	recipient, err := url2.Parse(args[0])
	if err != nil {
		return "", err
	}

	//query the token precision and reformat amount argument into a bigInt.
	amt, err := amountToBigInt(originUrl.String(), args[1])
	if err != nil {
		return "", err
	}

	params := protocol.IssueTokens{}
	params.Recipient = recipient
	params.Amount.Set(amt)

	return dispatchTxAndPrintResponse(&params, originUrl, signer)
}

func BurnTokens(origin string, args []string) (string, error) {
	originUrl, err := url2.Parse(origin)
	if err != nil {
		return "", err
	}

	args, signer, err := prepareSigner(originUrl, args)
	if err != nil {
		return "", err
	}

	if len(args) < 1 {
		return "", fmt.Errorf("amount to burn is not specified")
	}

	tokenUrl, err := GetTokenUrlFromAccount(originUrl)
	if err != nil {
		return "", fmt.Errorf("invalid token url was obtained from %s, %v", originUrl.String(), err)
	}

	//query the token precision and reformat amount argument into a bigInt.
	amt, err := amountToBigInt(tokenUrl.String(), args[0])
	if err != nil {
		return "", err
	}

	params := protocol.BurnTokens{}
	params.Amount.Set(amt)

	return dispatchTxAndPrintResponse(&params, originUrl, signer)
}

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	bookCmd.AddCommand(
		bookGetCmd,
		bookCreateCmd,
	)

	// Add auth cmd for backwards compatability
	bookCmd.AddCommand(authCmd)

}

// bookCmd are the commands associated with managing key books
var bookCmd = &cobra.Command{
	Use:   "book",
	Short: "Manage key books for a ADI chains",
}

var bookGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Deprecated - use `accumulate get ...`",
	Args:  cobra.ExactArgs(1),
	Run:   runCmdFunc(GetAndPrintKeyBook),
}

var bookCreateCmd = &cobra.Command{
	Use:   "create [origin adi url] [key name[@key book or page]] [new key book url] [public key 1 (optional)]",
	Short: "Create new key book and page. When public key 1 is specified it will be assigned to the page, otherwise the origin key is used.",
	Args:  cobra.RangeArgs(3, 4),
	Run:   runCmdFunc(CreateKeyBook),
}

func GetAndPrintKeyBook(args []string) (string, error) {
	res, _, err := GetKeyBook(args[0])
	if err != nil {
		return "", fmt.Errorf("error retrieving key book for %s", args[0])
	}

	return PrintChainQueryResponseV2(res)
}

func GetKeyBook(url string) (*QueryResponse, *protocol.KeyBook, error) {
	res, err := GetUrl(url)
	if err != nil {
		return nil, nil, err
	}

	if res.Type != protocol.AccountTypeKeyBook.String() {
		return nil, nil, fmt.Errorf("expecting key book but received %v", res.Type)
	}

	kb := protocol.KeyBook{}
	err = Remarshal(res.Data, &kb)
	if err != nil {
		return nil, nil, err
	}
	return res, &kb, nil
}

// CreateKeyBook create a new key book
func CreateKeyBook(args []string) (string, error) {
	originUrl, err := url2.Parse(args[0])
	if err != nil {
		return "", err
	}
	originKeyName := args[1]

	args, signer, err := prepareSigner(originUrl, args[1:])
	if err != nil {
		return "", err
	}
	if len(args) < 1 {
		return "", fmt.Errorf("invalid number of arguments")
	}

	newUrl, err := url2.Parse(args[0])
	if err != nil {
		return "", err
	}
	if newUrl.Authority != originUrl.Authority {
		return "", fmt.Errorf("the authority of book url to create (%s) doesn't match the origin adi's authority (%s)", newUrl.Authority, originUrl.Authority)
	}

	keyBook := protocol.CreateKeyBook{}
	keyBook.Url = newUrl

	for _, authUrlStr := range Authorities {
		authUrl, err := url2.Parse(authUrlStr)
		if err != nil {
			return "", err
		}
		keyBook.Authorities = append(keyBook.Authorities, authUrl)
	}

	var keyName string
	if len(args) > 1 {
		keyName = args[1]
	} else {
		keyName = originKeyName
	}
	k, err := resolvePublicKey(keyName)
	if err != nil {
		return "", fmt.Errorf("could not resolve public key hash %s: %w", keyName, err)
	}

	publicKeyHash := k.PublicKeyHash()
	keyBook.PublicKeyHash = publicKeyHash
	return dispatchTxAndPrintResponse(&keyBook, originUrl, signer)
}

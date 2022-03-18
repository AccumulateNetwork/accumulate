package cmd

import (
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/spf13/cobra"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// bookCmd are the commands associated with managing key books
var bookCmd = &cobra.Command{
	Use:   "book",
	Short: "Manage key books for a ADI chains",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if len(args) > 0 {
			switch args[0] {
			case "get":
				if len(args) > 0 {
					out, err = GetAndPrintKeyBook(args[1])
				} else {
					PrintKeyBookGet()
				}
			case "create":
				if len(args) > 3 {
					if args[0] == "create" {
						out, err = CreateKeyBook(args[1], args[2:])
					} else {
						fmt.Println("Usage:")
						PrintKeyBookCreate()
					}
				} else {
					fmt.Println("Usage:")
					PrintKeyBook()
				}
			default:
				PrintKeyBook()
			}
		} else {
			PrintKeyBook()
		}
		printOutput(cmd, out, err)
	},
}

func PrintKeyBookGet() {
	fmt.Println("  accumulate book get [URL]			Get existing Key Book by URL")
}

func PrintKeyBookCreate() {
	fmt.Println("  accumulate book create [origin adi url] [signing key name] [key index (optional)] [key height (optional)] [new key book url] [public key 1 (optional)] ... [public key hex or name n + 1] Create new key book and page. When public key 1 is specified it will be assigned to the page, otherwise the origin key is used.")
	fmt.Println("\t\t example usage: accumulate book create acc://RedWagon redKey5 acc://RedWagon/RedBook")
}

func PrintKeyBook() {
	PrintKeyBookGet()
	PrintKeyBookCreate()
}

func GetAndPrintKeyBook(url string) (string, error) {
	res, _, err := GetKeyBook(url)
	if err != nil {
		return "", fmt.Errorf("error retrieving key book for %s", url)
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
func CreateKeyBook(origin string, args []string) (string, error) {
	originUrl, err := url2.Parse(origin)
	if err != nil {
		return "", err
	}
	originKeyName := args[0]

	args, signer, err := prepareSigner(originUrl, args)
	if err != nil {
		return "", err
	}
	if len(args) < 1 {
		return "", fmt.Errorf("invalid number of arguments")
	}

	newUrl, err := url2.Parse(args[0])
	if newUrl.Authority != originUrl.Authority {
		return "", fmt.Errorf("the authority of book url to create (%s) doesn't match the origin adi's authority (%s)", newUrl.Authority, originUrl.Authority)
	}

	keyBook := protocol.CreateKeyBook{}
	keyBook.Url = newUrl

	var keyName string
	if len(args) > 1 {
		keyName = args[1]
	} else {
		keyName = originKeyName
	}
	publicKeyHash, err := resolvePublicKey(keyName)
	if err != nil {
		return "", fmt.Errorf("could not resolve public key hash %s: %w", keyName, err)
	}
	keyBook.PublicKeyHash = publicKeyHash

	res, err := dispatchTxRequest("create-key-book", &keyBook, nil, originUrl, signer)
	if err != nil {
		return "", err
	}

	if !TxNoWait && TxWait > 0 {
		_, err := waitForTxn(res.TransactionHash, TxWait)
		if err != nil {
			var rpcErr jsonrpc2.Error
			if errors.As(err, &rpcErr) {
				return PrintJsonRpcError(err)
			}
			return "", err
		}
	}
	return ActionResponseFrom(res).Print()
}

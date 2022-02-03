package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
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
	fmt.Println("  accumulate book create [origin adi url] [signing key name] [key index (optional)] [key height (optional)] [new key book url] [key page url 1] ... [key page url n + 1] Create new key book and assign key pages 1 to N+1 to the book")
	fmt.Println("\t\t example usage: accumulate book create acc://RedWagon redKey5 acc://RedWagon/RedBook acc://RedWagon/RedPage1")
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

	if res.Type != types.AccountTypeKeyBook.String() {
		return nil, nil, fmt.Errorf("expecting key book but received %v", res.Type)
	}

	kb := protocol.KeyBook{}
	err = Remarshal(res.Data, &kb)
	if err != nil {
		return nil, nil, err
	}
	return res, &kb, nil
}

// CreateKeyBook create a new key page
func CreateKeyBook(book string, args []string) (string, error) {
	bookUrl, err := url2.Parse(book)
	if err != nil {
		return "", err
	}

	args, si, privKey, err := prepareSigner(bookUrl, args)
	if err != nil {
		return "", err
	}
	if len(args) < 2 {
		return "", fmt.Errorf("invalid number of arguments")
	}

	newUrl, err := url2.Parse(args[0])

	if newUrl.Authority != bookUrl.Authority {
		return "", fmt.Errorf("book url to create (%s) doesn't match the authority adi (%s)", newUrl.Authority, bookUrl.Authority)
	}

	keyBook := protocol.CreateKeyBook{}
	keyBook.Url = newUrl.String()

	pageUrls := args[1:]
	for i := range pageUrls {
		u2, err := url2.Parse(pageUrls[i])
		if err != nil {
			return "", fmt.Errorf("invalid page url %s, %v", pageUrls[i], err)
		}
		keyBook.Pages = append(keyBook.Pages, u2.String())
	}

	res, err := dispatchTxRequest("create-key-book", &keyBook, nil, bookUrl, si, privKey)
	if err != nil {
		return "", err
	}
	return ActionResponseFrom(res).Print()
}

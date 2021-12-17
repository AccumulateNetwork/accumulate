package cmd

import (
	"bytes"
	"fmt"

	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/spf13/cobra"
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
	fmt.Println("  accumulate book create [actor adi url] [signing key name] [key index (optional)] [key height (optional)] [new key book url] [key page url 1] ... [key page url n + 1] Create new key book and assign key pages 1 to N+1 to the book")
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

	return PrintQueryResponseV2(res)
}

func GetKeyBook(url string) (*api2.QueryResponse, *protocol.KeyBook, error) {
	res, err := GetUrl(url)
	if err != nil {
		return nil, nil, err
	}

	v, ok := res.Data.(protocol.KeyBook)
	if !ok {
		return nil, nil, fmt.Errorf("returned data is not a key book for %v", url)
	}

	return res, &v, nil
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

	var chainId types.Bytes32
	pageUrls := args[1:]
	for i := range pageUrls {
		u2, err := url2.Parse(pageUrls[i])
		if err != nil {
			return "", fmt.Errorf("invalid page url %s, %v", pageUrls[i], err)
		}
		chainId.FromBytes(u2.ResourceChain())
		keyBook.Pages = append(keyBook.Pages, chainId)
	}

	res, err := dispatchTxRequest("create-key-book", &keyBook, bookUrl, si, privKey)
	if err != nil {
		return "", err
	}
	return ActionResponseFrom(res).Print()
}

func GetKeyPageInBook(book string, keyLabel string) (*protocol.KeyPage, int, error) {

	b, err := url2.Parse(book)
	if err != nil {
		return nil, 0, err
	}

	privKey, err := LookupByLabel(keyLabel)
	if err != nil {
		return nil, 0, err
	}

	_, kb, err := GetKeyBook(b.String())
	if err != nil {
		return nil, 0, err
	}

	for i := range kb.Pages {
		v := kb.Pages[i]
		//we have a match so go fetch the ssg
		s, err := GetByChainId(v[:])
		if err != nil {
			return nil, 0, err
		}
		if s.Type != types.ChainTypeKeyPage.String() {
			return nil, 0, fmt.Errorf("expecting key page, received %s", s.Type)
		}
		ss, ok := s.Data.(protocol.KeyPage)
		if !ok {
			return nil, 0, fmt.Errorf("returned chain is not a key page type")
		}

		for j := range ss.Keys {
			_, err := LookupByPubKey(ss.Keys[j].PublicKey)
			if err == nil && bytes.Equal(privKey[32:], v[:]) {
				return &ss, j, nil
			}
		}
	}

	return nil, 0, fmt.Errorf("key page not found in book %s for key name %s", book, keyLabel)
}

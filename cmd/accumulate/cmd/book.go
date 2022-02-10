package cmd

/*
	This file centers around creation and manipulation of key books via
	the Cobra command "book" and its subcommands.

	This file is part of a client application.

	For more information, see TODO: create database structure docs
*/

import (
	"errors"
	"fmt"

	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
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
	fmt.Println("  accumulate book create [origin adi url] [signing key name] [key index (optional)] [key height (optional)] [new key book url] [key page url 1] ... [key page url n + 1] Create new key book and assign key pages 1 to N+1 to the book")
	fmt.Println("\t\t example usage: accumulate book create acc://RedWagon redKey5 acc://RedWagon/RedBook acc://RedWagon/RedPage1")
}

func PrintKeyBook() {
	PrintKeyBookGet()
	PrintKeyBookCreate()
}

// TODO: SUGGEST: This method doesn't actually print anything to system output.
// Instead it prepares a string which is intended to be printed.
// Consider renaming.
func GetAndPrintKeyBook(url string) (string, error) {
	res, _, err := GetKeyBook(url)
	if err != nil {
		return "", fmt.Errorf("error retrieving key book for %s", url)
	}

	return PrintChainQueryResponseV2(res)
}

// Get the key book at the specified URL.
func GetKeyBook(url string) (*QueryResponse, *protocol.KeyBook, error) {
	data, err := GetUrl(url)
	if err != nil {
		return nil, nil, err
	}

	if data.Type != protocol.AccountTypeKeyBook.String() {
		return nil, nil, fmt.Errorf("expecting key book but received %v", data.Type)
	}

	kb := protocol.KeyBook{}
	err = Remarshal(data.Data, &kb)
	if err != nil {
		return nil, nil, err
	}
	return data, &kb, nil
}

// Create a new key page in the book located at the specified URL.
func CreateKeyBook(book string, args []string) (string, error) {
	bookUrl, err := url2.Parse(book)
	if err != nil {
		return "", err
	}

	args, trxHeader, privKey, err := prepareSigner(bookUrl, args)
	if err != nil {
		return "", err
	}
	if len(args) < 2 {
		// Reminder: the number of args has by now been modified
		// by prepareSigner()
		return "", fmt.Errorf("invalid number of arguments")
	}

	newUrl, err := url2.Parse(args[0])
	if err != nil {
		return "", err
	}

	if newUrl.Authority != bookUrl.Authority {
		return "", fmt.Errorf("book url to create (%s) doesn't match the authority adi (%s)", newUrl.Authority, bookUrl.Authority)
	}

	keyBook := protocol.CreateKeyBook{}
	keyBook.Url = newUrl

	// All remaining arguments are parsed as URLs and added as
	// pages.
	//
	// SUGGEST: Why must the user enter the full URL for every single page?
	// Can't we infer the correct URLs from the (validated) URL of the book
	// we just created? Then the user just has to enter a list of names.
	// VERIFY: VULNERABILITY: ....actually, allowing the user to enter URLs
	// for these pages would allow them to create pages on ANY arbitrary book.
	// Shouldn't we actually forbid URLs here?
	// Are we checking for this on the server side?
	pageUrls := args[1:]
	for i := range pageUrls {
		u2, err := url2.Parse(pageUrls[i])
		if err != nil {
			return "", fmt.Errorf("invalid page url %s, %v", pageUrls[i], err)
		}
		keyBook.Pages = append(keyBook.Pages, u2)
	}

	res, err := dispatchTxRequest("create-key-book", &keyBook, nil, bookUrl, trxHeader, privKey)
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

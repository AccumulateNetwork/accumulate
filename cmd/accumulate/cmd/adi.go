package cmd

/*
	This file centers around creation and manipulation ADI's via the Cobra
	command "adi" and its subcommands.

	This file is part of a client application.

	ADI stands for Accumulate Digital Identity.
	===========================================

	An ADI represents a single entity (a user) which has zero or more assets.
	Note that nothing stops a single entity from having more than one ADI, but
	Accumulate would neither know nor care about that.

	All Accumulate assets (accounts, keys, etc), except for lite accounts, are
	associated with exactly one	ADI.

	For more information, see TODO: create database structure docs
*/

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/spf13/cobra"
	api2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

func init() {
	adiCmd.AddCommand(
		adiGetCmd,
		adiListCmd,
		adiDirectoryCmd,
		adiCreateCmd)
}

var adiCmd = &cobra.Command{
	Use:   "adi",
	Short: "Create and manage ADI",
}

var adiGetCmd = &cobra.Command{
	Use:   "get [url]",
	Short: "Get existing ADI by URL",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := GetADI(args[0])
		printOutput(cmd, out, err)
	},
}
var adiListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all ADIs", // VERIFY:
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		out, err := ListADIs()
		printOutput(cmd, out, err)
	},
}
var adiDirectoryCmd = &cobra.Command{
	Use:   "directory [url] [from] [to]",
	Short: "Get directory of URL's associated with an ADI with starting index and number of directories to receive",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := GetAdiDirectory(args[0], args[1], args[2])
		printOutput(cmd, out, err)
	},
}
var adiCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create new ADI",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 3 {
			PrintADICreate()
			return
		}
		out, err := NewADI(args[0], args[1:])
		printOutput(cmd, out, err)
	},
}

func PrintADICreate() {
	fmt.Println("  accumulate adi create [origin-lite-account] [adi url to create] [public-key or key name] [key-book-name (optional)] [key-page-name (optional)]  Create new ADI from lite token account")
	fmt.Println("  accumulate adi create [origin-adi-url] [wallet signing key name] [key index (optional)] [key height (optional)] [adi url to create] [public key or wallet key name] [key book url (optional)] [key page url (optional)] Create new ADI for another ADI")
}

// GetAdiDirectory begins execution of a query for everything that is stored
// in the specified directory.
//
// This method is called when a user enters the appropriate command from their
// CLI as defined in var adiDirectoryCmd.
//
// Returns the data at the specified URL.
// TODO: Deep dive into database indexes required: does this fail
// if the given URL points to something that is not a directory?
func GetAdiDirectory(origin string, start string, count string) (string, error) {
	originURL, err := url2.Parse(origin)
	if err != nil {
		return "", fmt.Errorf("invalid Accumulate URL")
	}

	startVal, err := strconv.ParseInt(start, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid start value")
	}

	countVal, err := strconv.ParseInt(count, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid count value")
	}
	if countVal < 1 {
		return "", fmt.Errorf("count must be greater than zero")
	}

	params := api2.DirectoryQuery{}
	params.Url = originURL
	params.Start = uint64(startVal)
	params.Count = uint64(countVal)
	params.Expand = true

	data, err := json.Marshal(&params)
	if err != nil {
		return "", err
	}

	var res api2.MultiResponse
	if err := Client.RequestAPIv2(context.Background(), "query-directory", json.RawMessage(data), &res); err != nil {
		ret, err := PrintJsonRpcError(err)
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("%v", ret)
	}

	return PrintMultiResponse(&res)
}

// GetAdi begins execution of a query for all information about the specified
// ADI.
//
// This method is called when a user enters the appropriate command from their
// CLI as defined in var adiGetCmd.
//
// Verifies that an ADI resides at the given Accumulate URL and prints
// everything about it.
func GetADI(url string) (string, error) {
	res, err := GetUrl(url)
	if err != nil {
		return "", err
	}

	if res.Type != protocol.AccountTypeIdentity.String() {
		return "", fmt.Errorf("expecting ADI chain but received %v", res.Type)
	}

	return PrintChainQueryResponseV2(res)
}

// NewADI() redirects here after validating the origin URL.
// SUGGEST: These functions don't appear to need to be separate.
func NewADIFromADISigner(originURL *url2.URL, args []string) (string, error) {

	// TODO: Perform arg count check before proceeding, min and max.

	var trxHeader *transactions.Header
	var privKey []byte
	var err error

	args, trxHeader, privKey, err = prepareSigner(originURL, args)
	if err != nil {
		return "", err
	}

	var newbornURLstring string
	var newbornBookName string
	var newbornPageName string

	//
	// Expected args at this point:
	//
	// args[0] - The URL for the new ADI being created.
	// args[1] - The public being assigned to the new ADI.
	//
	// args[2] - Optional key book name.
	// args[3] - Optional key page name, requires arg[2].
	//

	if len(args) < 2 {
		return "", fmt.Errorf("invalid number of arguments")
	}

	newbornURLstring = args[0]
	newbornURL, err := url2.Parse(newbornURLstring)
	if err != nil {
		return "", fmt.Errorf("invalid adi url %s, %v", newbornURLstring, err)
	}

	pubKey, err := resolvePublicKey(args[1])
	if err != nil {
		return "", err
	}

	// If the user supplied at least 3 arguments then the third is the
	// book name.
	if len(args) > 2 {
		// TODO: SUGGEST: QUESTION: VULNERABILITY:
		// Do we want to sanitize book name inputs?
		// Are we sanitizing them on the server side?
		// Are there no invalid names? What if the user provides a URL?
		newbornBookName = args[2]
	} else {
		// TODO: SUGGEST: Shouldn't we either:
		//   A. Make this a config option
		//   B. Show in command usage that this is the default
		//   C. Explicitly state during execution that we're using a default
		//      name
		//
		//   Recommend option C.
		newbornBookName = "book0"
	}

	if len(args) > 3 {
		// TODO: SUGGEST: QUESTION: VULNERABILITY:
		// Do we want to sanitize page name inputs?
		// Are we sanitizing them on the server side?
		// Are there no invalid names? What if the user provides a URL?
		newbornPageName = args[3]
	} else {
		// TODO: SUGGEST: Shouldn't we either:
		//   A. Make this a config option
		//   B. Show in command usage that this is the default
		//   C. Explicitly state during execution that we're using a default
		//      name
		//
		//   Recommend option C.
		newbornPageName = "page0"
	}

	birthRequest := protocol.CreateIdentity{}
	birthRequest.Url = newbornURL
	birthRequest.PublicKey = pubKey
	birthRequest.KeyBookName = newbornBookName
	birthRequest.KeyPageName = newbornPageName

	res, err := dispatchTxRequest("create-adi", &birthRequest, nil, originURL, trxHeader, privKey)
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

	ar := ActionResponseFrom(res)
	out, err := ar.Print()
	if err != nil {
		return "", err
	}

	//todo: turn around and query the ADI and store the results.
	err = Db.Put(BucketAdi, []byte(newbornURL.Authority), pubKey)
	if err != nil {
		return "", fmt.Errorf("DB: %v", err)
	}

	return out, nil
}

// NewADI begins execution of a request to create a new ADI.
//
// This method is called when a user enters the appropriate command from their
// CLI as defined in PrintADICreate().
func NewADI(origin string, params []string) (string, error) {

	// Before doing ANYTHING else, check that the origin URL is valid.
	originURL, err := url2.Parse(origin)
	if err != nil {
		return "", err
	}

	return NewADIFromADISigner(originURL, params[:])
}

// ListADIs begins execution of a query for all ADI's in the local memory
// database.
//
// This method is called when a user enters the appropriate command from their
// CLI as defined in var adiListCmd.
//
// Prepares a human-readable, multi-line list of all ADIs which exist
// in the memory database. VERIFY:
//
// TODO: SUGGEST: This function doesn't actually list anything to system
// output. Instead it prepares a string which is intended to be printed
// later. Consider renaming.
func ListADIs() (string, error) {
	b, err := Db.GetBucket(BucketAdi)
	if err != nil {
		return "", err
	}

	var out string
	for _, v := range b.KeyValueList {
		u, err := url2.Parse(string(v.Key))
		if err != nil {
			out += fmt.Sprintf("%s\t:\t%x \n", v.Key, v.Value)
		} else {
			lab, err := FindLabelFromPubKey(v.Value)
			if err != nil { // TODO: VERIFY: I think these cases are backwards.
				out += fmt.Sprintf("%v\t:\t%x \n", u, v.Value)
			} else {
				out += fmt.Sprintf("%v\t:\t%s \n", u, lab)
			}
		}
	}
	return out, nil
}

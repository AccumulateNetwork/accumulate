package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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
	Short: "Get existing ADI by URL",
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
	fmt.Println("  accumulate adi create [origin-lite-account] [adi url to create] [public-key or key name] [key-book-name (optional)] Create new ADI from lite token account. If key book name is not specified, the adi key book is used.")
	fmt.Println("  accumulate adi create [adi account url with credits] [wallet signing key name] [adi url to create] [public key or wallet key name] [key book url (optional)]  Create new ADI for another ADI")
}

func GetAdiDirectory(origin string, start string, count string) (string, error) {
	u, err := url2.Parse(origin)
	if err != nil {
		return "", err
	}

	st, err := strconv.ParseInt(start, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid start value")
	}

	ct, err := strconv.ParseInt(count, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid count value")
	}
	if ct < 1 {
		return "", fmt.Errorf("count must be greater than zero")
	}

	params := api.DirectoryQuery{}
	params.Url = u
	params.Start = uint64(st)
	params.Count = uint64(ct)
	params.Expand = true

	data, err := json.Marshal(&params)
	if err != nil {
		return "", err
	}

	var res api.MultiResponse
	if err := Client.RequestAPIv2(context.Background(), "query-directory", json.RawMessage(data), &res); err != nil {
		return PrintJsonRpcError(err)
	}

	return PrintMultiResponse(&res)
}

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

func NewADIFromADISigner(origin *url2.URL, args []string) (string, error) {
	args, signer, err := prepareSigner(origin, args)
	if err != nil {
		return "", err
	}

	var adiUrlStr string
	var bookUrlStr string

	//at this point :
	//args[0] should be the new adi you are creating
	//args[1] should be the public key you are assigning to the adi
	//args[2] is an optional setting for the key book name
	//args[3] is an optional setting for the key page name
	//Note: if args[2] is not the keybook, the keypage also cannot be specified.
	if len(args) == 0 {
		return "", fmt.Errorf("insufficient number of command line arguments")
	}

	if len(args) > 0 {
		adiUrlStr = args[0]
	}
	if len(args) < 1 {
		return "", fmt.Errorf("invalid number of arguments")
	}

	var k *Key
	if len(args) > 1 {
		k, err = resolvePublicKey(args[1])
		if err != nil {
			return "", err
		}
	}

	adiUrl, err := url2.Parse(adiUrlStr)
	if err != nil {
		return "", fmt.Errorf("invalid adi url %s, %v", adiUrlStr, err)
	}

	var bookUrl *url2.URL
	if len(args) > 2 {
		bookUrlStr = args[2]

		bookUrl, err = url2.Parse(bookUrlStr)
		if err != nil {
			return "", fmt.Errorf("invalid book url %s, %v", bookUrlStr, err)
		}
	} else if adiUrl.IsRootIdentity() || k != nil {
		bookUrl = adiUrl.JoinPath("/book")
	}

	idc := protocol.CreateIdentity{}
	idc.Url = adiUrl
	idc.KeyBookUrl = bookUrl
	if k != nil {
		idc.KeyHash = k.PublicKeyHash()
	}

	for _, authUrlStr := range Authorities {
		authUrl, err := url2.Parse(authUrlStr)
		if err != nil {
			return "", err
		}
		idc.Authorities = append(idc.Authorities, authUrl)
	}

	out, err := dispatchTxAndPrintResponse(&idc, origin, signer)
	if err != nil {
		return PrintJsonRpcError(err)
	}

	//todo: turn around and query the ADI and store the results.
	if k != nil {
		err = GetWallet().Put(BucketAdi, []byte(adiUrl.Authority), k.PublicKey)
		if err != nil {
			return "", fmt.Errorf("DB: %v", err)
		}
	}

	return out, nil
}

// NewADI create a new ADI from a sponsored account.
func NewADI(origin string, params []string) (string, error) {

	u, err := url2.Parse(origin)
	if err != nil {
		return "", err
	}

	return NewADIFromADISigner(u, params[:])
}

func ListADIs() (string, error) {
	b, err := GetWallet().GetBucket(BucketAdi)
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
			if err != nil {
				out += fmt.Sprintf("%v\t:\t%x \n", u, v.Value)
			} else {
				out += fmt.Sprintf("%v\t:\t%s\n", u, lab)
			}
		}
	}
	return out, nil
}

package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	acmeapi "github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/spf13/cobra"
)

var adiCmd = &cobra.Command{
	Use:   "adi",
	Short: "Create and manage ADI",
	Run: func(cmd *cobra.Command, args []string) {
		var out string
		var err error
		if len(args) > 0 {
			switch arg := args[0]; arg {
			case "get":
				if len(args) > 1 {
					out, err = GetADI(args[1])
				} else {
					fmt.Println("Usage:")
					PrintADIGet()
				}
			case "list":
				out, err = ListADIs()
			case "directory":
				if len(args) > 1 {
					out, err = GetAdiDirectory(args[1])
					if err != nil {
						PrintAdiDirectory()
					}
				} else {
					PrintAdiDirectory()
				}
			case "create":
				if len(args) > 3 {
					out, err = NewADI(args[1], args[2:])
				} else {
					fmt.Println("Usage:")
					PrintADICreate()
				}
			default:
				fmt.Println("Usage:")
				PrintADI()
			}
		} else {
			fmt.Println("Usage:")
			PrintADI()
		}
		printOutput(cmd, out, err)
	},
}

func PrintADIGet() {
	fmt.Println("  accumulate adi get [URL]			Get existing ADI by URL")
}

func PrintADICreate() {
	fmt.Println("  accumulate adi create [actor-lite-account] [adi url to create] [public-key or key name] [key-book-name (optional)] [key-page-name (optional)]  Create new ADI from lite account")
	fmt.Println("  accumulate adi create [actor-adi-url] [wallet signing key name] [key index (optional)] [key height (optional)] [adi url to create] [public key or wallet key name] [key book url (optional)] [key page url (optional)] Create new ADI for another ADI")
}

func PrintADIImport() {
	fmt.Println("  accumulate adi import [adi-url] [private-key]	Import Existing ADI")
}

func PrintAdiDirectory() {
	fmt.Println("  accumulate adi directory [url] 		Get directory of URL's associated with an ADI")
}

func GetAdiDirectory(actor string) (string, error) {

	u, err := url2.Parse(actor)
	if err != nil {
		return "", err
	}

	var res acmeapi.APIDataResponse

	params := acmeapi.APIRequestURL{}

	params.URL = types.String(u.String())

	if err := Client.Request(context.Background(), "get-directory", params, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	return PrintQueryResponse(&res)
}

func PrintADI() {
	PrintADIGet()
	PrintAdiDirectory()
	PrintADICreate()
	PrintADIImport()
}

func GetADI(url string) (string, error) {

	var res acmeapi.APIDataResponse

	params := acmeapi.APIRequestURL{}
	params.URL = types.String(url)

	if err := Client.Request(context.Background(), "adi", params, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	return PrintQueryResponse(&res)
}

func NewADIFromADISigner(actor *url2.URL, args []string) (string, error) {
	var si *transactions.SignatureInfo
	var privKey []byte
	var err error

	args, si, privKey, err = prepareSigner(actor, args)
	if err != nil {
		return "", err
	}

	var adiUrl string
	var book string
	var page string

	//at this point :
	//args[0] should be the new adi you are creating
	//args[1] should be the public key you are assigning to the adi
	//args[2] is an optional setting for the key book name
	//args[3] is an optional setting for the key page name
	//Note: if args[2] is not the keybook, the keypage also cannot be specified.
	if len(args) == 0 {
		return "", fmt.Errorf("insufficient number of command line arguments")
	}

	if len(args) > 1 {
		adiUrl = args[0]
	}
	if len(args) < 2 {
		return "", fmt.Errorf("invalid number of arguments")
	}

	pubKey, err := getPublicKey(args[1])
	if err != nil {
		pubKey, err = pubKeyFromString(args[1])
		if err != nil {
			return "", fmt.Errorf("key %s, does not exist in wallet, nor is it a valid public key", args[1])
		}
	}

	if len(args) > 2 {
		book = args[2]
	} else {
		book = "book0"
	}

	if len(args) > 3 {
		page = args[3]
	} else {
		page = "page0"
	}

	u, err := url2.Parse(adiUrl)
	if err != nil {
		return "", fmt.Errorf("invalid adi url %s, %v", adiUrl, err)
	}

	idc := &protocol.IdentityCreate{}
	idc.Url = u.Authority
	idc.PublicKey = pubKey
	idc.KeyBookName = book
	idc.KeyPageName = page

	data, err := json.Marshal(idc)
	if err != nil {
		return "", err
	}

	dataBinary, err := idc.MarshalBinary()
	if err != nil {
		return "", err
	}

	nonce := nonceFromTimeNow()
	params, err := prepareGenTx(data, dataBinary, actor, si, privKey, nonce)
	if err != nil {
		return "", err
	}

	var res acmeapi.APIDataResponse
	if err := Client.Request(context.Background(), "adi-create", params, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	ar := ActionResponse{}
	err = json.Unmarshal(*res.Data, &ar)
	if err != nil {
		return "", fmt.Errorf("error unmarshalling create adi result, %v", err)
	}
	out, err := ar.Print()
	if err != nil {
		return "", err
	}

	//todo: turn around and query the ADI and store the results.
	err = Db.Put(BucketAdi, []byte(u.Authority), pubKey)
	if err != nil {
		return "", fmt.Errorf("DB: %v", err)
	}

	return out, nil
}

// NewADI create a new ADI from a sponsored account.
func NewADI(actor string, params []string) (string, error) {

	u, err := url2.Parse(actor)
	if err != nil {
		return "", err
	}

	return NewADIFromADISigner(u, params[:])
}

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
			if err != nil {
				out += fmt.Sprintf("%v\t:\t%x \n", u, v.Value)
			} else {
				out += fmt.Sprintf("%v\t:\t%s \n", u, lab)
			}
		}
	}
	return out, nil
}

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	acmeapi "github.com/AccumulateNetwork/accumulate/types/api"
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
	str, _, err := GetKeyBook(url)
	if err != nil {
		return "", fmt.Errorf("error retrieving key book for %s", url)
	}

	res := acmeapi.APIDataResponse{}
	err = json.Unmarshal([]byte(str), &res)
	if err != nil {
		return "", err
	}
	return PrintQueryResponse(&res)
}

func GetKeyBook(url string) ([]byte, *protocol.SigSpecGroup, error) {
	s, err := GetUrl(url, "sig-spec-group")
	if err != nil {
		return nil, nil, err
	}

	res := acmeapi.APIDataResponse{}
	err = json.Unmarshal(s, &res)
	if err != nil {
		return nil, nil, err
	}

	//added for compatibility with v1
	//data := strings.ReplaceAll(string(*res.Data), "keyBook", "sigSpecGroup")
	ssg := protocol.SigSpecGroup{}
	err = json.Unmarshal(*res.Data, &ssg)
	if err != nil {
		return nil, nil, err
	}

	return s, &ssg, nil
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

	ssg := protocol.CreateSigSpecGroup{}
	ssg.Url = newUrl.String()

	var chainId types.Bytes32
	pageUrls := args[1:]
	for i := range pageUrls {
		u2, err := url2.Parse(pageUrls[i])
		if err != nil {
			return "", fmt.Errorf("invalid page url %s, %v", pageUrls[i], err)
		}
		chainId.FromBytes(u2.ResourceChain())
		ssg.SigSpecs = append(ssg.SigSpecs, chainId)
	}

	data, err := json.Marshal(&ssg)
	if err != nil {
		return "", err
	}

	dataBinary, err := ssg.MarshalBinary()
	if err != nil {
		return "", err
	}

	nonce := uint64(time.Now().Unix())
	params, err := prepareGenTx(data, dataBinary, bookUrl, si, privKey, nonce)
	if err != nil {
		return "", err
	}

	var res acmeapi.APIDataResponse
	if err := Client.Request(context.Background(), "create-sig-spec-group", params, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	ar := ActionResponse{}
	err = json.Unmarshal(*res.Data, &ar)
	if err != nil {
		return "", fmt.Errorf("error unmarshalling create key book result")
	}
	return ar.Print()
}

func GetKeyPageInBook(book string, keyLabel string) (*protocol.SigSpec, int, error) {

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

	for i := range kb.SigSpecs {
		v := kb.SigSpecs[i]
		//we have a match so go fetch the ssg
		s, err := GetByChainId(v[:])
		if err != nil {
			return nil, 0, err
		}
		if *s.Type.AsString() != types.ChainTypeKeyPage.Name() {
			return nil, 0, fmt.Errorf("expecting key page, received %s", s.Type)
		}
		ss := protocol.SigSpec{}
		err = ss.UnmarshalBinary(*s.Data)
		if err != nil {
			return nil, 0, err
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

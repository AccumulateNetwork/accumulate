package goaccumulate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
)

func GetAndPrintKeyBook(url string) (string, error) {
	str, _, err := GetKeyBook(url)
	if err != nil {
		return "", fmt.Errorf("error retrieving key book for %s", url)
	}

	res := api2.QueryResponse{}

	err = json.Unmarshal([]byte(str), &res)
	if err != nil {
		return "", err
	}
	return PrintQueryResponseV2(&res)
}

func GetKeyBook(url string) ([]byte, *protocol.KeyBook, error) {
	s, err := GetUrl(url, "query-key-index")
	if err != nil {
		return nil, nil, err
	}

	res := api2.QueryResponse{}

	err = json.Unmarshal(s, &res)
	if err != nil {
		return nil, nil, err
	}

	ssg := protocol.KeyBook{}

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

	ssg := protocol.CreateKeyBook{}
	ssg.Url = newUrl.String()

	var chainId types.Bytes32
	pageUrls := args[1:]
	for i := range pageUrls {
		u2, err := url2.Parse(pageUrls[i])
		if err != nil {
			return "", fmt.Errorf("invalid page url %s, %v", pageUrls[i], err)
		}
		chainId.FromBytes(u2.ResourceChain())
		ssg.Pages = append(ssg.Pages, chainId)
	}

	data, err := json.Marshal(&ssg)
	if err != nil {
		return "", err
	}

	dataBinary, err := ssg.MarshalBinary()
	if err != nil {
		return "", err
	}

	nonce := nonceFromTimeNow()
	params, err := prepareGenTxV2(data, dataBinary, bookUrl, si, privKey, nonce)
	if err != nil {
		return "", err
	}

	var res api2.TxResponse

	if err := Client.RequestV2(context.Background(), "create-key-book", params, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	return ActionResponseFrom(&res).Print()

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
		s, _ := GetByChainId(v[:])
		if s != nil {
			fmt.Println("found key page")
		}
		//	if *s != types.ChainTypeKeyPage.Name() {
		//		return nil, 0, fmt.Errorf("expecting key page, received %s", s.Type)
		//	}
		ss := protocol.KeyPage{}

		//	err = ss.UnmarshalBinary(*s.Data)
		//	if err != nil {
		//		return nil, 0, err
		//	}

		for j := range ss.Keys {
			_, err := LookupByPubKey(ss.Keys[j].PublicKey)
			if err == nil && bytes.Equal(privKey[32:], v[:]) {
				return &ss, j, nil
			}
		}
	}

	return nil, 0, fmt.Errorf("key page not found in book %s for key name %s", book, keyLabel)
}

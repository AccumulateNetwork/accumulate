package client

import (
	"context"
	"encoding/json"
	"fmt"

	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
)

func GetKeyBook(url string) (*api2.QueryResponse, *protocol.KeyBook, error) {
	res, err := GetUrl(url)
	if err != nil {
		return nil, nil, err
	}

	ssg := protocol.KeyBook{}

	err = UnmarshalQuery(res.Data, &ssg)
	if err != nil {
		return nil, nil, err
	}

	return res, &ssg, nil
}

// CreateKeyBook create a new key page
func CreateKeyBook(book string, args []string) (*api2.TxResponse, error) {

	bookUrl, err := url2.Parse(book)
	if err != nil {
		return nil, err
	}

	args, si, privKey, err := prepareSigner(bookUrl, args)
	if err != nil {
		return nil, err
	}
	if len(args) < 2 {
		return nil, fmt.Errorf("invalid number of arguments")
	}

	newUrl, err := url2.Parse(args[0])

	if newUrl.Authority != bookUrl.Authority {
		return nil, fmt.Errorf("book url to create (%s) doesn't match the authority adi (%s)", newUrl.Authority, bookUrl.Authority)
	}

	ssg := protocol.CreateKeyBook{}
	ssg.Url = newUrl.String()

	var chainId types.Bytes32
	pageUrls := args[1:]
	for i := range pageUrls {
		u2, err := url2.Parse(pageUrls[i])
		if err != nil {
			return nil, fmt.Errorf("invalid page url %s, %v", pageUrls[i], err)
		}
		chainId.FromBytes(u2.ResourceChain())
		ssg.Pages = append(ssg.Pages, chainId)
	}

	data, err := json.Marshal(&ssg)
	if err != nil {
		return nil, err
	}

	dataBinary, err := ssg.MarshalBinary()
	if err != nil {
		return nil, err
	}

	nonce := nonceFromTimeNow()
	params, err := prepareGenTxV2(data, dataBinary, bookUrl, si, privKey, nonce)
	if err != nil {
		return nil, err
	}

	var res api2.TxResponse

	if err := Client.RequestV2(context.Background(), "create-key-book", params, &res); err != nil {
		return nil, err
	}

	return &res, nil

}

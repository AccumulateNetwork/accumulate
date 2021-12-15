package goaccumulate

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/client"
	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

func GetAdiDirectory(actor string) (string, error) {

	u, err := url2.Parse(actor)
	if err != nil {
		return "", err
	}

	var res api2.QueryResponse
	params := api2.UrlQuery{}
	params.Url = u.String()

	data, err := json.Marshal(&params)
	if err != nil {
		return "", err
	}

	if err := client.NewAPIClient().RequestV2(context.Background(), "query-directory", json.RawMessage(data), &res); err != nil {
		return PrintJsonRpcError(err)
	}

	return PrintQueryResponseV2(&res)
}

func GetADI(url string) (string, error) {

	var res api2.QueryResponse

	params := api2.UrlQuery{}
	params.Url = url

	data, err := json.Marshal(&params)
	if err != nil {
		return "", err
	}

	if err := client.NewAPIClient().RequestV2(context.Background(), "query", json.RawMessage(data), &res); err != nil {
		return PrintJsonRpcError(err)
	}

	return PrintQueryResponseV2(&res)
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
	}

	if len(args) > 3 {
		page = args[3]
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
	params, err := prepareGenTxV2(data, dataBinary, actor, si, privKey, nonce)
	if err != nil {
		return "", err
	}

	var res api2.QueryResponse

	if err := client.NewAPIClient().RequestV2(context.Background(), "create-adi", params, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	ar := ActionResponse{}
	//	err = json.Unmarshal(*res.Data, &ar)
	//	if err != nil {
	//		return "", fmt.Errorf("error unmarshalling create adi result, %v", err)
	//	}
	out, err := ar.Print()
	if err != nil {
		return "", err
	}

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

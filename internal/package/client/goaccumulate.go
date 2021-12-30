package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/AccumulateNetwork/accumulate/client"
	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/mdp/qrterminal"
)

var (
	Client         = client.NewAPIClient()
	WantJsonOutput = false
	TxPretend      = false
)

func GetToken(url string) (*api2.QueryResponse, error) {
	res := new(api2.QueryResponse)

	u, err := url2.Parse(url)
	params := api2.UrlQuery{}
	params.Url = u.String()

	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	if err := Client.RequestV2(context.Background(), "query", json.RawMessage(data), &res); err != nil {
		return nil, err
	}

	return res, nil

}

func GetAccount(url string) (*api2.QueryResponse, error) {
	var rez api2.QueryResponse
	u, err := url2.Parse(url)

	params := api2.UrlQuery{}
	params.Url = u.String()

	data, err := json.Marshal(&params)
	if err != nil {
		return nil, err
	}

	if err := Client.RequestV2(context.Background(), "query", json.RawMessage(data), &rez); err != nil {
		return nil, fmt.Errorf("%v", err)
	}

	return &rez, nil
}

func QrAccount(s string) (string, error) {
	u, err := url2.Parse(s)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid Accumulate URL: %v\n", s, err)
	}

	b := bytes.NewBufferString("")
	qrterminal.GenerateWithConfig(u.String(), qrterminal.Config{
		Level:          qrterminal.M,
		Writer:         b,
		HalfBlocks:     true,
		BlackChar:      qrterminal.BLACK_BLACK,
		BlackWhiteChar: qrterminal.BLACK_WHITE,
		WhiteChar:      qrterminal.WHITE_WHITE,
		WhiteBlackChar: qrterminal.WHITE_BLACK,
		QuietZone:      2,
	})

	r, err := ioutil.ReadAll(b)
	return string(r), err
}

func CreateAccount(url string, args []string) (*api2.TxResponse, error) {
	actor, err := url2.Parse(url)
	if err != nil {
		return nil, err
	}

	args, si, privKey, err := prepareSigner(actor, args)
	if len(args) < 3 {
		return nil, fmt.Errorf("insufficient number of command line arguments")
	}

	accountUrl, err := url2.Parse(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid account url %s", args[0])
	}
	if actor.Authority != accountUrl.Authority {
		return nil, fmt.Errorf("account url to create (%s) doesn't match the authority adi (%s)", accountUrl.Authority, actor.Authority)
	}
	tok, err := url2.Parse(args[1])
	if err != nil {
		return nil, fmt.Errorf("invalid token url")
	}

	var keybook string
	if len(args) > 2 {
		kbu, err := url2.Parse(args[2])
		if err != nil {
			return nil, fmt.Errorf("invalid key book url")
		}
		keybook = kbu.String()
	}

	tokenJson, err := Get(tok.String())
	if err != nil {
		return nil, err
	}
	token := protocol.TokenIssuer{}
	if err := json.Unmarshal(tokenJson, &token); err != nil {
		return nil, err
	}
	tac := &protocol.TokenAccountCreate{}
	tac.Url = accountUrl.String()
	tac.TokenUrl = tok.String()
	tac.KeyBookUrl = keybook

	jsonData, err := json.Marshal(tac)
	if err != nil {
		return nil, err
	}

	binaryData, err := tac.MarshalBinary()
	if err != nil {
		return nil, err
	}

	nonce := nonceFromTimeNow()
	params, err := prepareGenTxV2(jsonData, binaryData, actor, si, privKey, nonce)
	if err != nil {
		return nil, err
	}

	res := new(api2.TxResponse)

	if err := Client.RequestV2(context.Background(), "create-token-account", params, &res); err != nil {
		return nil, err
	}

	return res, nil
}

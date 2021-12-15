package goaccumulate

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/AccumulateNetwork/accumulate/client"
	"github.com/AccumulateNetwork/accumulate/cmd/cli/db"
	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/mdp/qrterminal"
)

var (
	Client         = client.NewAPIClient()
	Db             db.DB
	WantJsonOutput = false
	TxPretend      = false
)

var (
	BucketAnon     = []byte("anon")
	BucketAdi      = []byte("adi")
	BucketKeys     = []byte("keys")
	BucketLabel    = []byte("label")
	BucketMnemonic = []byte("mnemonic")
)

// -------------------------------------------------------------------------------
// Tokens
// -------------------------------------------------------------------------------

func GetToken(url string) {
	var res api2.QueryResponse

	params := api2.UrlQuery{}
	params.Url = url

	data, err := json.Marshal(&params)
	if err != nil {
		log.Fatal(err)
	}

	if err := Client.RequestV2(context.Background(), "query", json.RawMessage(data), &res); err != nil {
		log.Fatal(err)
	}

	str, err := json.Marshal(res)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(str))

}

func CreateToken(url string, symbol string, precision string, signer string) {
	fmt.Println("Creating new token " + symbol)
}

// -------------------------------------------------------------------------------
// Accounts
// -------------------------------------------------------------------------------

func GetAccount(url string) (string, error) {
	var rez api2.QueryResponse
	u, err := url2.Parse(url)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid Accumulate URL: %v\n", url, err)
	}

	params := api2.UrlQuery{}
	params.Url = u.String()

	data, err := json.Marshal(&params)
	if err != nil {
		return "", err
	}

	if err := Client.Request(context.Background(), "query", json.RawMessage(data), &rez); err != nil {
		return PrintJsonRpcError(err)
	}

	return PrintQueryResponseV2(&rez)
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

func CreateAccount(url string, args []string) (string, error) {
	actor, err := url2.Parse(url)
	if err != nil {
		return "", err
	}

	args, si, privKey, err := prepareSigner(actor, args)
	if len(args) < 3 {
		return "", fmt.Errorf("insufficient number of command line arguments")
	}

	accountUrl, err := url2.Parse(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid account url %s", args[0])
	}
	if actor.Authority != accountUrl.Authority {
		return "", fmt.Errorf("account url to create (%s) doesn't match the authority adi (%s)", accountUrl.Authority, actor.Authority)
	}
	tok, err := url2.Parse(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid token url")
	}

	var keybook string
	if len(args) > 2 {
		kbu, err := url2.Parse(args[2])
		if err != nil {
			return "", fmt.Errorf("invalid key book url")
		}
		keybook = kbu.String()
	}

	tokenJson, err := Get(tok.String())
	if err != nil {
		return "", err
	}
	token := protocol.TokenIssuer{}
	err = json.Unmarshal([]byte(tokenJson), &token)
	if err != nil {
		return "", fmt.Errorf("invalid token type %v", err)
	}

	tac := &protocol.TokenAccountCreate{}
	tac.Url = accountUrl.String()
	tac.TokenUrl = tok.String()
	tac.KeyBookUrl = keybook

	jsonData, err := json.Marshal(tac)
	if err != nil {
		return "", err
	}

	binaryData, err := tac.MarshalBinary()
	if err != nil {
		return "", err
	}

	nonce := nonceFromTimeNow()
	params, err := prepareGenTxV2(jsonData, binaryData, actor, si, privKey, nonce)
	if err != nil {
		return "", err
	}

	var res api2.TxResponse

	if err := Client.RequestV2(context.Background(), "create-token-account", params, &res); err != nil {
		return PrintJsonRpcError(err)
	}

	return ActionResponseFrom(&res).Print()
}

func GenerateAccount() (string, error) {
	return GenerateKey("")
}

func ListAccounts() (string, error) {

	b, err := Db.GetBucket(BucketLabel)
	if err != nil {
		//no accounts so nothing to do...
		return "", err
	}
	var out string
	for _, v := range b.KeyValueList {
		lt, err := protocol.LiteAddress(v.Value, protocol.AcmeUrl().String())
		if err != nil {
			continue
		}
		if lt.String() == string(v.Key) {
			out += fmt.Sprintf("%s\n", v.Key)
		}
	}
	return out, nil
}

func RestoreAccounts() (out string, err error) {
	anon, err := Db.GetBucket(BucketAnon)
	if err != nil {
		//no anon accounts so nothing to do...
		return
	}
	for _, v := range anon.KeyValueList {
		u, err := url2.Parse(string(v.Key))
		if err != nil {
			out += fmt.Sprintf("%q is not a valid URL\n", v.Key)
		}
		key, _, err := protocol.ParseLiteAddress(u)
		if err != nil {
			out += fmt.Sprintf("%q is not a valid lite account: %v\n", v.Key, err)
		} else if key == nil {
			out += fmt.Sprintf("%q is not a lite account\n", v.Key)
		}

		privKey := ed25519.PrivateKey(v.Value)
		pubKey := privKey.Public().(ed25519.PublicKey)
		out += fmt.Sprintf("Converting %s : %x\n", v.Key, pubKey)

		err = Db.Put(BucketLabel, v.Key, pubKey)
		if err != nil {
			log.Fatal(err)
		}
		err = Db.Put(BucketKeys, pubKey, privKey)
		if err != nil {
			return "", err
		}
		err = Db.DeleteBucket(BucketAnon)
		if err != nil {
			return "", err
		}
	}
	return out, nil
}

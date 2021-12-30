package client

import (
	"bytes"
	"context"
	"encoding"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"math/big"
	"strconv"
	"time"

	api2 "github.com/AccumulateNetwork/accumulate/internal/api/v2"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	acmeapi "github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/AccumulateNetwork/accumulate/types/api/query"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type Prep struct {
	actor *url2.URL
	args  []string
}

func getRecord(url string, rec interface{}) (*api2.MerkleState, error) {
	params := api2.UrlQuery{
		Url: url,
	}
	res := new(api2.QueryResponse)
	res.Data = rec
	if err := Client.RequestV2(context.Background(), "query", &params, res); err != nil {
		return nil, err
	}
	return res.MerkleState, nil
}

func getRecordById(chainId []byte, rec interface{}) (*api2.MerkleState, error) {
	params := api2.ChainIdQuery{
		ChainId: chainId,
	}
	res := new(api2.QueryResponse)
	res.Data = rec
	if err := Client.RequestV2(context.Background(), "query-chain", &params, res); err != nil {
		return nil, err
	}
	return res.MerkleState, nil
}

func prepareSigner(actor *url2.URL, args []string) ([]string, *transactions.SignatureInfo, []byte, error) {
	var privKey []byte
	var err error

	ct := 0
	if len(args) == 0 {
		return nil, nil, nil, fmt.Errorf("insufficent arguments on comand line")
	}

	ed := transactions.SignatureInfo{}
	ed.URL = actor.String()
	ed.KeyPageHeight = 1
	ed.KeyPageIndex = 0

	if len(args) > 1 {
		b, err := pubKeyFromString(args[0])
		if err != nil {
			privKey, err = LookupByLabel(args[0])
			if err != nil {
				return nil, nil, nil, fmt.Errorf("invalid public key or wallet label specified on command line")
			}

		} else {
			privKey, err = LookupByPubKey(b)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("invalid public key, cannot resolve signing key")
			}
		}
		ct++
	} else {
		return nil, nil, nil, fmt.Errorf("insufficent arguments on comand line")
	}

	if len(args) > 2 {
		if v, err := strconv.ParseInt(args[1], 10, 64); err == nil {
			ct++
			ed.KeyPageIndex = uint64(v)
		}
	}

	actorRec := new(state.ChainHeader)
	_, err = getRecord(actor.String(), &actorRec)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get %q : %v", actor, err)
	}

	bookRec := new(protocol.KeyBook)
	if actorRec.KeyBook == (types.Bytes32{}) {
		_, err := getRecord(actor.String(), &bookRec)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get %q : %v", actor, err)
		}
	} else {
		_, err := getRecordById(actorRec.KeyBook[:], &bookRec)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get %q : %v", actor, err)
		}
	}

	if ed.KeyPageIndex >= uint64(len(bookRec.Pages)) {
		return nil, nil, nil, fmt.Errorf("key page index %d is out of bound of the key book of %q", ed.KeyPageIndex, actor)
	}
	ms, err := getRecordById(bookRec.Pages[ed.KeyPageIndex][:], nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get chain %x : %v", bookRec.Pages[ed.KeyPageIndex][:], err)
	}
	ed.KeyPageHeight = ms.Count

	return args[ct:], &ed, privKey, nil
}

func signGenTx(binaryPayload []byte, origin *url2.URL, si *transactions.SignatureInfo, privKey []byte, nonce uint64) (*transactions.ED25519Sig, error) {
	gtx := new(transactions.GenTransaction)
	gtx.Transaction = binaryPayload

	gtx.ChainID = origin.ResourceChain()
	gtx.Routing = origin.Routing()

	si.Nonce = nonce
	gtx.SigInfo = si

	ed := new(transactions.ED25519Sig)
	err := ed.Sign(nonce, privKey, gtx.TransactionHash())
	if err != nil {
		return nil, err
	}
	return ed, nil
}

func prepareGenTxV2(jsonPayload, binaryPayload []byte, origin *url2.URL, si *transactions.SignatureInfo, privKey []byte, nonce uint64) (*api2.TxRequest, error) {
	ed, err := signGenTx(binaryPayload, origin, si, privKey, nonce)
	if err != nil {
		return nil, err
	}

	params := &api2.TxRequest{}

	if TxPretend {
		params.CheckOnly = true
	}

	// TODO The payload field can be set equal to the struct, without marshalling first
	params.Payload = json.RawMessage(jsonPayload)
	params.Signer.PublicKey = privKey[32:]
	params.Signer.Nonce = nonce
	params.Origin = origin.String()
	params.KeyPage.Height = si.KeyPageHeight
	params.KeyPage.Index = si.KeyPageIndex

	params.Signature = ed.GetSignature()
	//The public key needs to be used to verify the signature, however,
	//to pass verification, the validator will hash the key and check the
	//sig spec group to make sure this key belongs to the identity.
	params.Signer.PublicKey = ed.GetPublicKey()

	return params, err
}

func GetUrl(url string) (*api2.QueryResponse, error) {
	var res api2.QueryResponse

	u, err := url2.Parse(url)
	params := api2.UrlQuery{}
	params.Url = u.String()

	data, err := json.Marshal(&params)
	if err != nil {
		return nil, err
	}

	if err := Client.RequestV2(context.Background(), "query", json.RawMessage(data), &res); err != nil {
		return nil, fmt.Errorf("%v", err)
	}

	return &res, nil
}

type KeyPageStore struct {
	PrivKeys []types.Bytes `json:"privKeys"`
}

type KeyBookStore struct {
	KeyPageList []string `json:"keyPages"`
}

type AccountKeyBookStore struct {
	KeyBook KeyBookStore `json:"keyBook"`
}

func dispatchRequest(action string, payload interface{}, actor *url2.URL, si *transactions.SignatureInfo, privKey []byte) (interface{}, error) {
	json.Marshal(payload)

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	dataBinary, err := payload.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		return nil, err
	}

	nonce := nonceFromTimeNow()
	params, err := prepareGenTxV2(data, dataBinary, actor, si, privKey, nonce)
	if err != nil {
		return nil, err
	}

	var res interface{}
	if err := Client.RequestV2(context.Background(), "create-key-book", params, &res); err != nil {
		return nil, err
	}

	return res, nil
}

type ActionResponse struct {
	Txid      types.Bytes32 `json:"txid"`
	Hash      types.Bytes32 `json:"hash"`
	Log       types.String  `json:"log"`
	Code      types.String  `json:"code"`
	Codespace types.String  `json:"codespace"`
	Error     types.String  `json:"error"`
	Mempool   types.String  `json:"mempool"`
}

type ActionDataResponse struct {
	EntryHash types.Bytes32 `json:"entryHash"`
	ActionResponse
}

func ActionResponseFromData(r *api2.TxResponse, entryHash []byte) *ActionDataResponse {
	ar := &ActionDataResponse{}
	ar.EntryHash.FromBytes(entryHash)
	ar.ActionResponse = *ActionResponseFrom(r)
	return ar
}

func (a *ActionDataResponse) Print() (string, error) {
	var out string
	if WantJsonOutput {
		ok := a.Code == "0" || a.Code == ""
		if ok {
			a.Code = "ok"
		}
		b, err := json.Marshal(a)
		if err != nil {
			return "", err
		}
		out = string(b)
	} else {
		s, err := a.ActionResponse.Print()
		if err != nil {
			return "", err
		}
		out = fmt.Sprintf("\n\tEntry Hash\t\t:%x\n%s", a.EntryHash[:], s[1:])
	}
	return out, nil
}

func ActionResponseFrom(r *api2.TxResponse) *ActionResponse {
	return &ActionResponse{
		Txid:  types.Bytes(r.Txid).AsBytes32(),
		Hash:  r.Hash,
		Error: types.String(r.Message),
		Code:  types.String(fmt.Sprint(r.Code)),
	}
}

func (a *ActionResponse) Print() (string, error) {
	ok := a.Code == "0" || a.Code == ""

	var out string
	if WantJsonOutput {
		if ok {
			a.Code = "ok"
		}
		b, err := json.Marshal(a)
		if err != nil {
			return "", err
		}
		out = string(b)
	} else {
		out += fmt.Sprintf("\n\tTransaction Identifier\t:\t%x\n", a.Txid)
		out += fmt.Sprintf("\tTendermint Reference\t:\t%x\n", a.Hash)
		if !ok {
			out += fmt.Sprintf("\tError code\t\t:\t%s\n", a.Code)
		} else {
			out += fmt.Sprintf("\tError code\t\t:\tok\n")
		}
		if a.Error != "" {
			out += fmt.Sprintf("\tError\t\t\t:\t%s\n", a.Error)
		}
		if a.Log != "" {
			out += fmt.Sprintf("\tLog\t\t\t:\t%s\n", a.Log)
		}
		if a.Codespace != "" {
			out += fmt.Sprintf("\tCodespace\t\t:\t%s\n", a.Codespace)
		}
	}

	if ok {
		return out, nil
	}
	return "", errors.New(out)
}

var (
	ApiToString = map[string]string{
		"liteTokenAccount": "lite account",
		"tokenAccount":     "ADI token account",
		"adi":              "ADI",
		"keyBook":          "Key Book",
		"keyPage":          "Key Page",
	}
)

func UnmarshalQuery(src interface{}, d interface{}) error {
	ds, err := json.Marshal(src)
	if err != nil {
		return err
	}

	err = json.Unmarshal(ds, d)
	if err != nil {
		return err
	}

	return nil
}

func formatAmount(tokenUrl string, amount *big.Int) (string, error) {

	//query the token
	tokenData, err := Get(tokenUrl)
	if err != nil {
		return "", fmt.Errorf("error retrieving token url, %v", err)
	}
	r := acmeapi.APIDataResponse{}

	t := protocol.TokenIssuer{}
	err = json.Unmarshal(*r.Data, &t)
	if err != nil {
		return "", err
	}

	bf := big.Float{}
	bd := big.Float{}
	bd.SetFloat64(math.Pow(10.0, float64(t.Precision)))
	bf.SetInt(amount)
	bal := big.Float{}
	bal.Quo(&bf, &bd)

	return fmt.Sprintf("%s %s", bal.String(), t.Symbol), nil
}

func resolveKeyPageUrl(adi string, chainId []byte) (string, error) {
	var res acmeapi.APIDataResponse
	params := acmeapi.APIRequestURL{}
	params.URL = types.String(adi)
	if err := Client.RequestV2(context.Background(), "query-directory", params, &res); err != nil {
		return err, nil
	}

	dqr := protocol.DirectoryQueryResult{}
	err := json.Unmarshal(*res.Data, &dqr)
	if err != nil {
		return "", err
	}

	for _, s := range dqr.Entries {
		u, err := url2.Parse(s)
		if err != nil {
			continue
		}

		if bytes.Equal(u.ResourceChain(), chainId) {
			return s, nil
		}
	}

	return fmt.Sprintf("unresolvable chain %x", chainId), nil
}

func nonceFromTimeNow() uint64 {
	t := time.Now()
	return uint64(t.Unix()*1e6) + uint64(t.Nanosecond())/1e3
}

func pubKeyFromString(s string) ([]byte, error) {
	var pubKey types.Bytes32
	if len(s) != 64 {
		return nil, fmt.Errorf("invalid public key or wallet key name")
	}
	i, err := hex.Decode(pubKey[:], []byte(s))

	if err != nil {
		return nil, err
	}

	if i != 32 {
		return nil, fmt.Errorf("invalid public key")
	}

	return pubKey[:], nil
}

func GetByChainId(chainId []byte) (*api2.QueryResponse, error) {
	var res api2.QueryResponse
	res.Data = new(query.ResponseByChainId)

	params := api2.ChainIdQuery{}
	params.ChainId = chainId

	data, err := json.Marshal(&params)
	if err != nil {
		return nil, err
	}

	if err := Client.RequestV2(context.Background(), "query-chain", json.RawMessage(data), &res); err != nil {
		log.Fatal(err)
	}

	//return PrintQueryResponseV2(res)
	return &res, nil
}

func Get(accountUrl string) (*api2.QueryResponse, error) {
	u, err := url2.Parse(accountUrl)
	if err != nil {
		return nil, err
	}

	var res api2.QueryResponse

	params := api2.UrlQuery{}
	params.Url = u.String()

	data, err := json.Marshal(&params)
	if err != nil {
		return nil, err
	}
	if err := Client.RequestV2(context.Background(), "query", json.RawMessage(data), &res); err != nil {
		return nil, err
	}

	return &res, nil
}

func GetKey(url, key string) (*api2.QueryResponse, error) {
	var res api2.QueryResponse
	keyb, err := hex.DecodeString(key)
	if err != nil {
		return nil, err
	}

	params := api2.KeyPageIndexQuery{}
	params.Url = url
	params.Key = keyb

	data, err := json.Marshal(&params)
	if err != nil {
		return nil, err
	}

	err = Client.RequestV2(context.Background(), "query-key-index", json.RawMessage(data), &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

package cmd

import (
	"bytes"
	"context"
	"encoding"
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
	"github.com/AccumulateNetwork/accumulate/types/api/response"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/AccumulateNetwork/accumulate/types/synthetic"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/spf13/cobra"
)

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
	//adiActor labelOrPubKeyHex height index
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

	if IsLiteAccount(actor.String()) == true {
		privKey, err = LookupByLabel(actor.String()) //LookupByAnon(actor.String())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to find private key for lite account %s %v", actor.String(), err)
		}
		return args, &ed, privKey, nil
	}

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

func signGenTx(binaryPayload []byte, actor *url2.URL, si *transactions.SignatureInfo, privKey []byte, nonce uint64) (*transactions.ED25519Sig, error) {
	gtx := new(transactions.GenTransaction)
	gtx.Transaction = binaryPayload

	gtx.ChainID = actor.ResourceChain()
	gtx.Routing = actor.Routing()

	si.Nonce = nonce
	gtx.SigInfo = si

	ed := new(transactions.ED25519Sig)
	err := ed.Sign(nonce, privKey, gtx.TransactionHash())
	if err != nil {
		return nil, err
	}
	return ed, nil
}

func prepareGenTx(jsonPayload []byte, binaryPayload []byte, actor *url2.URL, si *transactions.SignatureInfo, privKey []byte, nonce uint64) (*acmeapi.APIRequestRaw, error) {
	ed, err := signGenTx(binaryPayload, actor, si, privKey, nonce)
	if err != nil {
		return nil, err
	}

	params := &acmeapi.APIRequestRaw{}
	params.Tx = &acmeapi.APIRequestRawTx{}

	params.Tx.Data = &json.RawMessage{}
	*params.Tx.Data = jsonPayload
	params.Tx.Signer = &acmeapi.Signer{}
	params.Tx.Signer.PublicKey.FromBytes(privKey[32:])
	params.Tx.Signer.Nonce = nonce
	params.Tx.Sponsor = types.String(actor.String())
	params.Tx.KeyPage = &acmeapi.APIRequestKeyPage{}
	params.Tx.KeyPage.Height = si.KeyPageHeight
	params.Tx.KeyPage.Index = si.KeyPageIndex

	params.Tx.Sig = types.Bytes64{}

	params.Tx.Sig.FromBytes(ed.GetSignature())
	//The public key needs to be used to verify the signature, however,
	//to pass verification, the validator will hash the key and check the
	//sig spec group to make sure this key belongs to the identity.
	params.Tx.Signer.PublicKey.FromBytes(ed.GetPublicKey())

	return params, err
}

func prepareGenTxV2(jsonPayload, binaryPayload []byte, actor *url2.URL, si *transactions.SignatureInfo, privKey []byte, nonce uint64) (*api2.TxRequest, error) {
	ed, err := signGenTx(binaryPayload, actor, si, privKey, nonce)
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
	params.Sponsor = actor.String()
	params.KeyPage.Height = si.KeyPageHeight
	params.KeyPage.Index = si.KeyPageIndex

	params.Signature = ed.GetSignature()
	//The public key needs to be used to verify the signature, however,
	//to pass verification, the validator will hash the key and check the
	//sig spec group to make sure this key belongs to the identity.
	params.Signer.PublicKey = ed.GetPublicKey()

	return params, err
}

func IsLiteAccount(url string) bool {
	u, err := url2.Parse(url)
	if err != nil {
		log.Fatal(err)
	}
	u2, err := url2.Parse(u.Authority)
	if err != nil {
		log.Fatal(err)
	}
	return protocol.IsValidAdiUrl(u2) != nil
}

func GetUrl(url string, method string) ([]byte, error) {

	var res interface{}
	var str []byte

	u, err := url2.Parse(url)
	params := acmeapi.APIRequestURL{}
	params.URL = types.String(u.String())

	if err := Client.Request(context.Background(), method, params, &res); err != nil {
		ret, err := PrintJsonRpcError(err)
		return []byte(ret), err
	}

	str, err = json.Marshal(res)
	if err != nil {
		return nil, err
	}

	return str, nil
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

//
//func (a *AdiStore) MarshalBinary() ([]byte, error) {
//	var buf bytes.Buffer
//
//	buf.Write(common.Uint64Bytes(uint64(len(a.tokenAccounts))))
//	for i := range a.tokenAccounts {
//		buf.Write(common.SliceBytes([]byte(a.tokenAccounts[i])))
//	}
//
//	buf.Write(common.Uint64Bytes(uint64(len(a.keyBooks))))
//	for i := range a.keyBooks {
//		buf.Write(common.SliceBytes([]byte(a.keyBooks[i])))
//	}
//
//	return buf.Bytes(), nil
//}
//
//func (a *AdiStore) UnmarshalBinary(data []byte) (err error) {
//	defer func() {
//		if rErr := recover(); rErr != nil {
//			err = fmt.Errorf("insufficent data to unmarshal AdiStore %v", rErr)
//		}
//	}()
//
//	var s []byte
//	l, data := common.BytesUint64(data)
//	for i := uint64(0); i < l; i++ {
//		s, data = common.BytesSlice(data)
//		a.tokenAccounts[i] = string(s)
//	}
//
//	l, data = common.BytesUint64(data)
//	for i := uint64(0); i < l; i++ {
//		s, data = common.BytesSlice(data)
//		a.keyBooks[i] = string(s)
//	}
//
//	return nil
//}

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
	params, err := prepareGenTx(data, dataBinary, actor, si, privKey, nonce)
	if err != nil {
		return nil, err
	}

	var res interface{}
	if err := Client.Request(context.Background(), "create-sig-spec-group", params, &res); err != nil {
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

func PrintJsonRpcError(err error) (string, error) {
	var e jsonrpc2.Error
	switch err := err.(type) {
	case jsonrpc2.Error:
		e = err
	default:
		return "", fmt.Errorf("error with request, %v", err)
	}

	if WantJsonOutput {
		out, err := json.Marshal(e)
		if err != nil {
			return "", err
		}
		return "", errors.New(string(out))
	} else {
		var out string
		out += fmt.Sprintf("\n\tMessage\t\t:\t%v\n", e.Message)
		out += fmt.Sprintf("\tError Code\t:\t%v\n", e.Code)
		out += fmt.Sprintf("\tDetail\t\t:\t%s\n", e.Data)
		return "", errors.New(out)
	}
}

func printOutput(cmd *cobra.Command, out string, err error) {
	if err != nil {
		cmd.Print("Error: ")
		cmd.PrintErr(err)
		cmd.Println()
		DidError = true
	} else {
		cmd.Println(out)
	}
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

func formatAmount(tokenUrl string, amount *big.Int) (string, error) {

	//query the token
	tokenData, err := Get(tokenUrl)
	if err != nil {
		return "", fmt.Errorf("error retrieving token url, %v", err)
	}
	r := acmeapi.APIDataResponse{}
	err = json.Unmarshal([]byte(tokenData), &r)
	if err != nil {
		return "", err
	}

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

func printGeneralTransactionParameters(res *acmeapi.APIDataResponse) string {
	out := fmt.Sprintf("---\n")
	out += fmt.Sprintf("  - Transaction           : %x\n", res.TxId.AsBytes32())
	out += fmt.Sprintf("  - Signer Url            : %s\n", res.Sponsor)
	out += fmt.Sprintf("  - Signature             : %x\n", res.Sig.Bytes())
	if res.Signer != nil {
		out += fmt.Sprintf("  - Signer Key            : %x\n", res.Signer.PublicKey.Bytes())
		out += fmt.Sprintf("  - Signer Nonce          : %d\n", res.Signer.Nonce)
	}
	out += fmt.Sprintf("  - Key Page              : %d (height) / %d (index)\n", res.KeyPage.Height, res.KeyPage.Index)
	out += fmt.Sprintf("===\n")
	return out
}

func PrintQueryResponseV2(v2 *api2.QueryResponse) (string, error) {
	if WantJsonOutput {
		data, err := json.Marshal(v2)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	v1 := new(acmeapi.APIDataResponse)
	v1.Type = types.String(v2.Type)
	if v2.MerkleState != nil {
		v1.MerkleState = new(acmeapi.MerkleState)
		v1.MerkleState.Count = v2.MerkleState.Count
		v1.MerkleState.Roots = make([]types.Bytes, len(v2.MerkleState.Roots))
		for i, r := range v2.MerkleState.Roots {
			v1.MerkleState.Roots[i] = r
		}
	}
	v1.Sponsor = types.String(v2.Sponsor)
	if v2.KeyPage != nil {
		v1.KeyPage = new(acmeapi.APIRequestKeyPage)
		v1.KeyPage.Height = v2.KeyPage.Height
		v1.KeyPage.Index = v2.KeyPage.Index
	}
	v1.TxId = (*types.Bytes)(&v2.Txid)
	if v2.Signer != nil {
		v1.Signer = new(acmeapi.Signer)
		v1.Signer.PublicKey = types.Bytes(v2.Signer.PublicKey).AsBytes32()
		v1.Signer.Nonce = v2.Signer.Nonce
	}
	sig := types.Bytes(v2.Sig).AsBytes64()
	v1.Sig = &sig

	b, err := json.Marshal(v2.Data)
	if err != nil {
		return "", err
	}
	v1.Data = (*json.RawMessage)(&b)

	b, err = json.Marshal(v2.Status)
	if err != nil {
		return "", err
	}
	v1.Status = (*json.RawMessage)(&b)

	out, err := PrintQueryResponse(v1)
	if err != nil {
		return "", err
	}

	for i, txid := range v2.SyntheticTxids {
		out += fmt.Sprintf("  - Synthetic Transaction %d : %x\n", i, txid)
	}
	return out, nil
}

func PrintQueryResponse(res *acmeapi.APIDataResponse) (string, error) {
	if WantJsonOutput {
		data, err := json.Marshal(res)
		if err != nil {
			return "", err
		}
		return string(data), nil
	} else {
		switch res.Type {
		case "liteTokenAccount":
			ata := response.LiteTokenAccount{}
			err := json.Unmarshal(*res.Data, &ata)
			if err != nil {
				return "", err
			}

			amt, err := formatAmount(ata.TokenUrl, &ata.Balance.Int)
			if err != nil {
				amt = "unknown"
			}

			var out string
			out += fmt.Sprintf("\n\tAccount Url\t:\t%v\n", ata.Url)
			out += fmt.Sprintf("\tToken Url\t:\t%v\n", ata.TokenUrl)
			out += fmt.Sprintf("\tBalance\t\t:\t%s\n", amt)
			out += fmt.Sprintf("\tCredits\t\t:\t%s\n", ata.CreditBalance.String())
			out += fmt.Sprintf("\tNonce\t\t:\t%d\n", ata.Nonce)

			return out, nil
		case "tokenAccount":
			ata := response.TokenAccount{}
			err := json.Unmarshal(*res.Data, &ata)
			if err != nil {
				return "", err
			}

			amt, err := formatAmount(ata.TokenUrl, &ata.Balance.Int)
			if err != nil {
				amt = "unknown"
			}
			var out string
			out += fmt.Sprintf("\n\tAccount Url\t:\t%v\n", ata.Url)
			out += fmt.Sprintf("\tToken Url\t:\t%v\n", ata.TokenUrl)
			out += fmt.Sprintf("\tBalance\t\t:\t%s\n", amt)
			out += fmt.Sprintf("\tKey Book Url\t:\t%s\n", ata.KeyBookUrl)

			return out, nil
		case "adi":
			adi := response.ADI{}
			err := json.Unmarshal(*res.Data, &adi)
			if err != nil {
				return "", err
			}

			var out string
			out += fmt.Sprintf("\n\tADI Url\t\t:\t%v\n", adi.Url)
			out += fmt.Sprintf("\tKey Book Url\t:\t%s\n", adi.KeyBookName)

			return out, nil
		case "directory":
			dqr := protocol.DirectoryQueryResult{}
			err := json.Unmarshal(*res.Data, &dqr)
			if err != nil {
				return "", err
			}
			var out string
			out += fmt.Sprintf("\n\tADI Entries\n")
			for _, s := range dqr.Entries {
				data, err := Get(s)
				if err != nil {
					return "", err
				}
				r := acmeapi.APIDataResponse{}
				err = json.Unmarshal([]byte(data), &r)

				chainType := "unknown"
				if err == nil {
					if v, ok := ApiToString[*r.Type.AsString()]; ok {
						chainType = v
					}
				}
				out += fmt.Sprintf("\t%v (%s)\n", s, chainType)
			}
			return out, nil
		case "keyBook":
			//workaround for protocol unmarshaling bug
			var ssg struct {
				Type      types.ChainType `json:"type" form:"type" query:"type" validate:"required"`
				ChainUrl  types.String    `json:"url" form:"url" query:"url" validate:"required,alphanum"`
				SigSpecId []byte          `json:"sigSpecId"` //this is the chain id for the sig spec for the chain
				SigSpecs  []types.Bytes32 `json:"sigSpecs"`
			}

			err := json.Unmarshal(*res.Data, &ssg)
			if err != nil {
				return "", err
			}

			u, err := url2.Parse(*ssg.ChainUrl.AsString())
			if err != nil {
				return "", err
			}
			var out string
			out += fmt.Sprintf("\n\tHeight\t\tKey Page Url\n")
			for i, v := range ssg.SigSpecs {
				//enable this code when testnet updated to a version > 0.2.1.
				//data, err := GetByChainId(v[:])
				//keypage := "unknown"
				//
				//if err == nil {
				//	r := acmeapi.APIDataResponse{}
				//	err = json.Unmarshal(*data.Data, &r)
				//	if err == nil {
				//		ss := protocol.KeyPage{}
				//		err = json.Unmarshal(*r.Data, &ss)
				//		keypage = *ss.ChainUrl.AsString()
				//	}
				//}
				//out += fmt.Sprintf("\t%d\t\t:\t%s\n", i, keypage)
				//hack to resolve the keypage url given the chainid
				s, err := resolveKeyPageUrl(u.Authority, v[:])
				if err != nil {
					return "", err
				}
				out += fmt.Sprintf("\t%d\t\t:\t%s\n", i+1, s)
			}
			return out, nil
		case "keyPage":
			ss := protocol.KeyPage{}
			err := json.Unmarshal(*res.Data, &ss)
			if err != nil {
				return "", err
			}

			out := fmt.Sprintf("\n\tIndex\tNonce\tPublic Key\t\t\t\t\t\t\t\tKey Name\n")
			for i, k := range ss.Keys {
				keyName := ""
				name, err := FindLabelFromPubKey(k.PublicKey)
				if err == nil {
					keyName = name
				}
				out += fmt.Sprintf("\t%d\t%d\t%x\t%s", i, k.Nonce, k.PublicKey, keyName)
			}
			return out, nil
		case "withdrawTokens":
			tx := response.TokenTx{}
			err := json.Unmarshal(*res.Data, &tx)
			if err != nil {
				return "", fmt.Errorf("cannot extract token transaction data from request")
			}

			var out string
			for i := range tx.ToAccount {
				bi := big.Int{}
				bi.SetInt64(int64(tx.ToAccount[i].Amount))
				amt, err := formatAmount("acc://ACME", &bi)
				if err != nil {
					amt = "unknown"
				}
				out += fmt.Sprintf("Send %s from %s to %s\n", amt, *tx.From.AsString(), tx.ToAccount[i].URL.String)
				out += fmt.Sprintf("  - Synthetic Transaction : %x\n", tx.ToAccount[i].SyntheticTxId)
			}

			out += printGeneralTransactionParameters(res)
			return out, nil
		case "syntheticDepositTokens":
			deposit := synthetic.TokenTransactionDeposit{}
			err := json.Unmarshal(*res.Data, &deposit)

			if err != nil {
				return "", err
			}

			out := "\n"
			amt, err := formatAmount(*deposit.TokenUrl.AsString(), &deposit.DepositAmount.Int)
			if err != nil {
				amt = "unknown"
			}
			out += fmt.Sprintf("Receive %s from %s to %s\n", amt, *deposit.FromUrl.AsString(),
				*deposit.ToUrl.AsString())

			out += printGeneralTransactionParameters(res)
			return out, nil

		default:
			return "", fmt.Errorf("unknown response type %q", res.Type)
		}
	}
}

func resolveKeyPageUrl(adi string, chainId []byte) (string, error) {
	var res acmeapi.APIDataResponse
	params := acmeapi.APIRequestURL{}
	params.URL = types.String(adi)
	if err := Client.Request(context.Background(), "get-directory", params, &res); err != nil {
		return PrintJsonRpcError(err)
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

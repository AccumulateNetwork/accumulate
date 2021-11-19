package cmd

import (
	"bytes"
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"github.com/AccumulateNetwork/accumulate/types/synthetic"
	"log"
	"math"
	"math/big"
	"os"
	"strconv"
	"time"

	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	acmeapi "github.com/AccumulateNetwork/accumulate/types/api"
	"github.com/AccumulateNetwork/accumulate/types/api/response"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
)

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
	ed.MSHeight = 1
	ed.PriorityIdx = 0

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
			ed.PriorityIdx = uint64(v)
			if len(args) > 3 {
				if v, err := strconv.ParseInt(args[2], 10, 64); err == nil {
					ct++
					ed.MSHeight = uint64(v)
				}
			}
		}
	}

	return args[ct:], &ed, privKey, nil
}

func prepareGenTx(jsonPayload []byte, binaryPayload []byte, actor *url2.URL, si *transactions.SignatureInfo, privKey []byte, nonce uint64) (*acmeapi.APIRequestRaw, error) {

	params := &acmeapi.APIRequestRaw{}
	params.Tx = &acmeapi.APIRequestRawTx{}

	params.Tx.Data = &json.RawMessage{}
	*params.Tx.Data = jsonPayload
	params.Tx.Signer = &acmeapi.Signer{}
	params.Tx.Signer.PublicKey.FromBytes(privKey[32:])
	params.Tx.Signer.Nonce = nonce
	params.Tx.Sponsor = types.String(actor.String())
	params.Tx.KeyPage = &acmeapi.APIRequestKeyPage{}
	params.Tx.KeyPage.Height = si.MSHeight
	params.Tx.KeyPage.Index = si.PriorityIdx

	params.Tx.Sig = types.Bytes64{}

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
	params.Tx.Sig.FromBytes(ed.GetSignature())
	//The public key needs to be used to verify the signature, however,
	//to pass verification, the validator will hash the key and check the
	//sig spec group to make sure this key belongs to the identity.
	params.Tx.Signer.PublicKey.FromBytes(ed.GetPublicKey())

	gtx.Signature = append(gtx.Signature, ed)

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
		log.Fatal(err)
	}

	str, err = json.Marshal(res)
	if err != nil {
		log.Fatal(err)
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
		log.Fatal(err)
	}

	dataBinary, err := payload.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		log.Fatal(err)
	}

	nonce := uint64(time.Now().Unix())
	params, err := prepareGenTx(data, dataBinary, actor, si, privKey, nonce)
	if err != nil {
		log.Fatal(err)
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

func (a *ActionResponse) Print() {
	if WantJsonOutput {
		if a.Code == "0" || a.Code == "" {
			a.Code = "ok"
		}
		dump, err := json.Marshal(a)
		if err != nil {
			log.Fatal(err)
		}
		log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))
		log.Fatal(string(dump))
	} else {
		var out string
		out += fmt.Sprintf("\n\tTransaction Identifier\t:\t%x\n", a.Txid)
		out += fmt.Sprintf("\tTendermint Reference\t:\t%x\n", a.Hash)
		if a.Code != "0" && a.Code != "" {
			out += fmt.Sprintf("\tError code\t\t:\t%s\n", a.Code)
		} else {
			out += fmt.Sprintf("\tError code\t\t:\tok\n")
		}
		if a.Error != "" {
			out += fmt.Sprintf("\tError\t\t:\t%s\n", a.Error)
		}
		if a.Log != "" {
			out += fmt.Sprintf("\tLog\t\t\t:\t%s\n", a.Log)
		}
		if a.Codespace != "" {
			out += fmt.Sprintf("\tCodespace\t\t:\t%s\n", a.Codespace)
		}
		log.Fatal(out)
	}
}

func PrintJsonRpcError(err error) {
	var e jsonrpc2.Error
	switch err.(type) {
	case jsonrpc2.Error:
		e = err.(jsonrpc2.Error)
	default:
		log.Fatalf("error with request, %v", err)
	}

	var out string
	if WantJsonOutput {
		dump, err := json.Marshal(e)
		if err != nil {
			log.Fatal(err)
		}
		log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))
		log.Fatal(string(dump))
	} else {
		out += fmt.Sprintf("\n\tMessage\t\t:\t%v\n", e.Message)
		out += fmt.Sprintf("\tError Code\t:\t%v\n", e.Code)
		out += fmt.Sprintf("\tDetail\t\t:\t%s\n", e.Data)
		log.Fatal(out)
	}
}

var (
	ApiToString = map[string]string{
		"anonTokenAccount": "lite account",
		"tokenAccount":     "ADI token account",
		"adi":              "ADI",
		"sigSpecGroup":     "Key Book",
		"sigSpec":          "Key Page",
	}
)

func formatAmount(tokenUrl string, amount *big.Int) (string, error) {

	//query the token
	tokenData := Get(tokenUrl)
	r := acmeapi.APIDataResponse{}
	err := json.Unmarshal([]byte(tokenData), &r)
	if err != nil {
		return "", err
	}

	t := response.Token{}
	err = json.Unmarshal(*r.Data, &t)
	if err != nil {
		log.Fatal(err)
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
	out += fmt.Sprintf("  - Merkle DAG Root       : %x\n", res.MDRoot.AsBytes32())
	out += fmt.Sprintf("  - Signer Url            : %s\n", res.Sponsor)
	out += fmt.Sprintf("  - Signature             : %x\n", res.Sig.Bytes())
	out += fmt.Sprintf("  - Signer Key            : %x\n", res.Signer.PublicKey.Bytes())
	out += fmt.Sprintf("  - Signer Nonce          : %d\n", res.Signer.Nonce)
	out += fmt.Sprintf("  - Key Page              : %d (height) / %d (index)\n", res.KeyPage.Height, res.KeyPage.Index)
	out += fmt.Sprintf("===\n")
	return out
}

func PrintQueryResponse(res *acmeapi.APIDataResponse) {
	if WantJsonOutput {
		data, err := json.Marshal(res)
		if err != nil {
			log.Fatal(err)
		}
		//log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))
		//log.Fatal(string(data))
		fmt.Fprintf(os.Stderr, string(data))
	} else {
		switch res.Type {
		case "anonTokenAccount":
			ata := response.AnonTokenAccount{}
			err := json.Unmarshal(*res.Data, &ata)
			if err != nil {
				log.Fatal(err)
			}

			//query the token
			tokenData := Get(ata.TokenUrl)
			r := acmeapi.APIDataResponse{}
			err = json.Unmarshal([]byte(tokenData), &r)
			if err != nil {
				log.Fatal(err)
			}
			t := response.Token{}
			err = json.Unmarshal(*r.Data, &t)
			if err != nil {
				log.Fatal(err)
			}

			bf := big.Float{}
			bd := big.Float{}
			bd.SetFloat64(math.Pow(10.0, float64(t.Precision)))
			bf.SetInt(&ata.Balance.Int)
			bal := big.Float{}
			bal.Quo(&bf, &bd)

			var out string
			out += fmt.Sprintf("\n\tAccount Url\t:\t%v\n", ata.Url)
			out += fmt.Sprintf("\tToken Url\t:\t%v\n", ata.TokenUrl)
			out += fmt.Sprintf("\tBalance\t\t:\t%s %s\n", bal.String(), t.Symbol)
			out += fmt.Sprintf("\tCredits\t\t:\t%s\n", ata.CreditBalance.String())
			out += fmt.Sprintf("\tNonce\t\t:\t%d\n", ata.Nonce)

			fmt.Fprintf(os.Stderr, string(out))
		case "tokenAccount":
			ata := response.TokenAccount{}
			err := json.Unmarshal(*res.Data, &ata)
			if err != nil {
				log.Fatal(err)
			}

			amt, err := formatAmount(ata.TokenUrl, &ata.Balance.Int)
			if err != nil {
				amt = "unknown"
			}
			var out string
			out += fmt.Sprintf("\n\tAccount Url\t:\t%v\n", ata.Url)
			out += fmt.Sprintf("\tToken Url\t:\t%v\n", ata.TokenUrl)
			out += fmt.Sprintf("\tBalance\t\t:\t%s %s\n", amt)
			out += fmt.Sprintf("\tKey Book Url\t:\t%s\n", ata.KeyBookUrl)

			fmt.Fprintf(os.Stderr, string(out))
		case "adi":
			adi := response.ADI{}
			err := json.Unmarshal(*res.Data, &adi)
			if err != nil {
				log.Fatal(err)
			}

			var out string
			out += fmt.Sprintf("\n\tADI Url\t\t:\t%v\n", adi.Url)
			out += fmt.Sprintf("\tKey Book Url\t:\t%s\n", adi.KeyBookName)

			fmt.Fprintf(os.Stderr, string(out))
		case "directory":
			dqr := protocol.DirectoryQueryResult{}
			err := json.Unmarshal(*res.Data, &dqr)
			if err != nil {
				log.Fatal(err)
			}
			var out string
			out += fmt.Sprintf("\n\tADI Entries\n")
			for _, s := range dqr.Entries {
				data := Get(s)
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
			fmt.Fprintf(os.Stderr, string(out))
		case "sigSpecGroup":
			//workaround for protocol unmarshaling bug
			var ssg struct {
				Type      types.ChainType `json:"type" form:"type" query:"type" validate:"required"`
				ChainUrl  types.String    `json:"url" form:"url" query:"url" validate:"required,alphanum"`
				SigSpecId []byte          `json:"sigSpecId"` //this is the chain id for the sig spec for the chain
				SigSpecs  []types.Bytes32 `json:"sigSpecs"`
			}

			err := json.Unmarshal(*res.Data, &ssg)
			if err != nil {
				log.Fatal(err)
			}

			u, err := url2.Parse(*ssg.ChainUrl.AsString())
			if err != nil {
				log.Fatal(err)
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
				//		ss := protocol.SigSpec{}
				//		err = json.Unmarshal(*r.Data, &ss)
				//		keypage = *ss.ChainUrl.AsString()
				//	}
				//}
				//out += fmt.Sprintf("\t%d\t\t:\t%s\n", i, keypage)
				//hack to resolve the keypage url given the chainid
				s := resolveKeyPageUrl(u.Authority, v[:])
				out += fmt.Sprintf("\t%d\t:\t%s\n", i+1, s)
			}
			fmt.Fprintf(os.Stderr, string(out))
		case "sigSpec":
			ss := protocol.SigSpec{}
			err := json.Unmarshal(*res.Data, &ss)
			if err != nil {
				log.Fatal(err)
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
			fmt.Fprintf(os.Stderr, string(out))
		case "tokenTx":
			tx := response.TokenTx{}
			err := json.Unmarshal(*res.Data, &tx)
			if err != nil {
				log.Fatalf("Cannot extract token transaction data from request")
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
			fmt.Fprintf(os.Stderr, string(out))
		case "syntheticTokenDeposit":
			deposit := synthetic.TokenTransactionDeposit{}
			err := json.Unmarshal(*res.Data, &deposit)

			if err != nil {
				log.Fatal(err)
			}

			out := "\n"
			amt, err := formatAmount(*deposit.TokenUrl.AsString(), &deposit.DepositAmount.Int)
			if err != nil {
				amt = "unknown"
			}
			out += fmt.Sprintf("Receive %s from %s to %s\n", amt, *deposit.FromUrl.AsString(),
				*deposit.ToUrl.AsString())

			out += printGeneralTransactionParameters(res)
			fmt.Fprintf(os.Stderr, string(out))

		default:

		}
	}
}

func resolveKeyPageUrl(adi string, chainId []byte) string {
	var res acmeapi.APIDataResponse
	params := acmeapi.APIRequestURL{}
	params.URL = types.String(adi)
	if err := Client.Request(context.Background(), "get-directory", params, &res); err != nil {
		PrintJsonRpcError(err)
	}

	dqr := protocol.DirectoryQueryResult{}
	err := json.Unmarshal(*res.Data, &dqr)
	if err != nil {
		log.Fatal(err)
	}

	for _, s := range dqr.Entries {
		u, err := url2.Parse(s)
		if err != nil {
			continue
		}

		if bytes.Equal(u.ResourceChain(), chainId) {
			return s
		}
	}

	return fmt.Sprintf("unresolvable chain %x", chainId)
}

package cmd

import (
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
	"github.com/AccumulateNetwork/accumulate/internal/url"
	url2 "github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/response"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/spf13/cobra"
)

func getRecord(url string, rec interface{}) (*api2.MerkleState, error) {
	params := api2.UrlQuery{
		Url: url,
	}
	res := new(api2.QueryResponse)
	res.Data = rec
	if err := Client.Request(context.Background(), "query", &params, res); err != nil {
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
	if err := Client.Request(context.Background(), "query-chain", &params, res); err != nil {
		return nil, err
	}
	return res.MerkleState, nil
}

func prepareSigner(origin *url2.URL, args []string) ([]string, *transactions.SignatureInfo, []byte, error) {
	var privKey []byte
	var err error

	ct := 0
	if len(args) == 0 {
		return nil, nil, nil, fmt.Errorf("insufficent arguments on comand line")
	}

	ed := transactions.SignatureInfo{}
	ed.URL = origin.String()
	ed.KeyPageHeight = 1
	ed.KeyPageIndex = 0

	if IsLiteAccount(origin.String()) == true {
		privKey, err = LookupByLabel(origin.String())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to find private key for lite account %s %v", origin.String(), err)
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

	originRec := new(state.ChainHeader)
	_, err = getRecord(origin.String(), &originRec)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get %q : %v", origin, err)
	}

	bookRec := new(protocol.KeyBook)
	if originRec.KeyBook == "" {
		_, err := getRecord(origin.String(), &bookRec)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get %q : %v", origin, err)
		}
	} else {
		_, err := getRecord(*originRec.KeyBook.AsString(), &bookRec)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get %q : %v", origin, err)
		}
	}

	if ed.KeyPageIndex >= uint64(len(bookRec.Pages)) {
		return nil, nil, nil, fmt.Errorf("key page index %d is out of bound of the key book of %q", ed.KeyPageIndex, origin)
	}
	u, err := url.Parse(bookRec.Pages[ed.KeyPageIndex])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid keypage url %s", bookRec.Pages[ed.KeyPageIndex])
	}
	ms, err := getRecordById(u.ResourceChain(), nil)
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

func UnmarshalQuery(src interface{}, dst interface{}) error {
	d, err := json.Marshal(src)
	if err != nil {
		return err
	}

	err = json.Unmarshal(d, dst)
	if err != nil {
		return err
	}

	return nil
}

func GetUrlAs(url string, as interface{}) error {
	res, err := GetUrl(url)
	if err != nil {
		return err
	}

	return UnmarshalQuery(res, as)
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

	if err := Client.Request(context.Background(), "query", json.RawMessage(data), &res); err != nil {
		ret, err := PrintJsonRpcError(err)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%v", ret)
	}

	return &res, nil
}

func dispatchTxRequest(action string, payload encoding.BinaryMarshaler, origin *url2.URL, si *transactions.SignatureInfo, privKey []byte) (*api2.TxResponse, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	dataBinary, err := payload.MarshalBinary()
	if err != nil {
		return nil, err
	}

	nonce := nonceFromTimeNow()
	params, err := prepareGenTxV2(data, dataBinary, origin, si, privKey, nonce)
	if err != nil {
		return nil, err
	}

	data, err = json.Marshal(params)
	if err != nil {
		return nil, err
	}

	var res api2.TxResponse
	if err := Client.Request(context.Background(), action, json.RawMessage(data), &res); err != nil {
		_, err := PrintJsonRpcError(err)
		return nil, err
	}

	return &res, nil
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
		out += fmt.Sprintf("\n\tTransaction Id\t\t:\t%x\n", a.Txid)
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

type JsonRpcError struct {
	Msg string
	Err jsonrpc2.Error
}

func (e *JsonRpcError) Error() string { return e.Msg }

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
		return "", &JsonRpcError{Err: e, Msg: string(out)}
	} else {
		var out string
		out += fmt.Sprintf("\n\tMessage\t\t:\t%v\n", e.Message)
		out += fmt.Sprintf("\tError Code\t:\t%v\n", e.Code)
		out += fmt.Sprintf("\tDetail\t\t:\t%s\n", e.Data)
		return "", &JsonRpcError{Err: e, Msg: out}
	}
}

func printOutput(cmd *cobra.Command, out string, err error) {
	if err != nil {
		cmd.PrintErrf("Error: %v\n", err)
		DidError = err
	} else {
		cmd.Println(out)
	}
}

var (
	ApiToString = map[types.ChainType]string{
		types.ChainTypeLiteTokenAccount: "Lite Account",
		types.ChainTypeTokenAccount:     "ADI Token Account",
		types.ChainTypeIdentity:         "ADI",
		types.ChainTypeKeyBook:          "Key Book",
		types.ChainTypeKeyPage:          "Key Page",
		types.ChainTypeDataAccount:      "Data Chain",
		types.ChainTypeLiteDataAccount:  "Lite Data Chain",
	}
)

func formatAmount(tokenUrl string, amount *big.Int) (string, error) {

	//query the token
	tokenData, err := Get(tokenUrl)
	if err != nil {
		return "", fmt.Errorf("error retrieving token url, %v", err)
	}
	t := protocol.TokenIssuer{}
	err = json.Unmarshal([]byte(tokenData), &t)
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

func printGeneralTransactionParameters(res *api2.QueryResponse) string {
	out := fmt.Sprintf("---\n")
	out += fmt.Sprintf("  - Transaction           : %x\n", res.Txid)
	out += fmt.Sprintf("  - Signer Url            : %s\n", res.Origin)
	out += fmt.Sprintf("  - Signature             : %x\n", res.Sig)
	if res.Signer != nil {
		out += fmt.Sprintf("  - Signer Key            : %x\n", res.Signer.PublicKey)
		out += fmt.Sprintf("  - Signer Nonce          : %d\n", res.Signer.Nonce)
	}
	out += fmt.Sprintf("  - Key Page              : %d (height) / %d (index)\n", res.KeyPage.Height, res.KeyPage.Index)
	out += fmt.Sprintf("===\n")
	return out
}

func PrintQueryResponseV2(v2 *api2.QueryResponse) (string, error) {
	if WantJsonOutput || v2.Type == "dataEntry" || v2.Type == "dataSet" {
		data, err := json.Marshal(v2)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	out, err := outputForHumans(v2)
	if err != nil {
		return "", err
	}

	for i, txid := range v2.SyntheticTxids {
		out += fmt.Sprintf("  - Synthetic Transaction %d : %x\n", i, txid)
	}
	return out, nil
}

func outputForHumans(res *api2.QueryResponse) (string, error) {
	switch string(res.Type) {
	case types.ChainTypeLiteTokenAccount.String():
		ata := protocol.LiteTokenAccount{}
		err := UnmarshalQuery(res.Data, &ata)
		if err != nil {
			return "", err
		}

		amt, err := formatAmount(ata.TokenUrl, &ata.Balance)
		if err != nil {
			amt = "unknown"
		}

		var out string
		out += fmt.Sprintf("\n\tAccount Url\t:\t%v\n", ata.ChainUrl)
		out += fmt.Sprintf("\tToken Url\t:\t%v\n", ata.TokenUrl)
		out += fmt.Sprintf("\tBalance\t\t:\t%s\n", amt)
		out += fmt.Sprintf("\tCredits\t\t:\t%s\n", ata.CreditBalance.String())
		out += fmt.Sprintf("\tNonce\t\t:\t%d\n", ata.Nonce)

		return out, nil
	case types.ChainTypeTokenAccount.String():
		ata := protocol.TokenAccount{}
		err := UnmarshalQuery(res.Data, &ata)
		if err != nil {
			return "", err
		}

		amt, err := formatAmount(ata.TokenUrl, &ata.Balance)
		if err != nil {
			amt = "unknown"
		}

		var out string
		out += fmt.Sprintf("\n\tAccount Url\t:\t%v\n", ata.ChainUrl)
		out += fmt.Sprintf("\tToken Url\t:\t%s\n", ata.TokenUrl)
		out += fmt.Sprintf("\tBalance\t\t:\t%s\n", amt)
		out += fmt.Sprintf("\tKey Book Url\t:\t%s\n", ata.KeyBook)

		return out, nil
	case types.ChainTypeIdentity.String():
		adi := state.AdiState{}
		err := UnmarshalQuery(res.Data, &adi)
		if err != nil {
			return "", err
		}

		var out string
		out += fmt.Sprintf("\n\tADI url\t\t:\t%v\n", adi.ChainUrl)
		out += fmt.Sprintf("\tKey Book url\t:\t%s\n", adi.KeyBook)

		return out, nil
	case "directory":
		dqr := api2.DirectoryQueryResult{}
		err := UnmarshalQuery(res.Data, &dqr)
		if err != nil {
			return "", err
		}

		var out string
		out += fmt.Sprintf("\n\tADI Entries: start = %d, count = %d, total = %d\n", dqr.Start, dqr.Count, dqr.Total)
		for _, s := range dqr.ExpandedEntries {
			header := state.ChainHeader{}
			err = UnmarshalQuery(s.Data, &header)
			if err != nil {
				return "", err
			}

			chainDesc := "unknown"
			if err == nil {
				if v, ok := ApiToString[header.Type]; ok {
					chainDesc = v
				}
			}
			out += fmt.Sprintf("\t%v (%s)\n", header.ChainUrl, chainDesc)
		}
		return out, nil
	case types.ChainTypeKeyBook.String():
		book := protocol.KeyBook{}
		err := UnmarshalQuery(res.Data, &book)
		if err != nil {
			return "", err
		}

		var out string
		out += fmt.Sprintf("\n\tPage Index\t\tKey Page Url\n")
		for i, v := range book.Pages {
			out += fmt.Sprintf("\t%d\t\t:\t%s\n", i, v)
		}
		return out, nil
	case types.ChainTypeKeyPage.String():
		ss := protocol.KeyPage{}
		err := UnmarshalQuery(res.Data, &ss)
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
	case types.TxTypeSendTokens.String():
		tx := response.TokenTx{}
		err := UnmarshalQuery(res.Data, &tx)
		if err != nil {
			return "", err
		}

		var out string
		for i := range tx.ToAccount {
			bi := big.Int{}
			bi.SetInt64(int64(tx.ToAccount[i].Amount))
			amt, err := formatAmount("acc://ACME", &bi)
			if err != nil {
				amt = "unknown"
			}
			out += fmt.Sprintf("Send %s from %s to %s\n", amt, *tx.From.AsString(), tx.ToAccount[i].URL)
			out += fmt.Sprintf("  - Synthetic Transaction : %x\n", tx.ToAccount[i].SyntheticTxId)
		}

		out += printGeneralTransactionParameters(res)
		return out, nil
	case types.TxTypeSyntheticDepositTokens.String():
		deposit := new(protocol.SyntheticDepositTokens)
		err := UnmarshalQuery(res.Data, &deposit)
		if err != nil {
			return "", err
		}

		out := "\n"
		amt, err := formatAmount(deposit.Token, &deposit.Amount)
		if err != nil {
			amt = "unknown"
		}
		out += fmt.Sprintf("Receive %s to %s (cause: %X)\n", amt, res.Origin, deposit.Cause)

		out += printGeneralTransactionParameters(res)
		return out, nil
	case types.TxTypeCreateIdentity.String():
		id := protocol.IdentityCreate{}
		err := UnmarshalQuery(res.Data, &id)
		if err != nil {
			return "", err
		}

		out := "\n"
		out += fmt.Sprintf("ADI url \t\t:\tacc://%s\n", id.Url)
		out += fmt.Sprintf("Key Book \t\t:\tacc://%s/%s\n", id.Url, id.KeyBookName)
		out += fmt.Sprintf("Key Page \t\t:\tacc://%s/%s\n", id.Url, id.KeyPageName)

		keyName, err := FindLabelFromPubKey(id.PublicKey)
		if err != nil {
			out += fmt.Sprintf("Public Key \t:\t%x\n", id.PublicKey)
		} else {
			out += fmt.Sprintf("Public Key (name) \t:\t%x (%s)\n", id.PublicKey, keyName)
		}

		out += printGeneralTransactionParameters(res)
		return out, nil

	default:
		return "", fmt.Errorf("unknown response type %q", res.Type)
	}
}

func getChainHeaderFromChainId(chainId []byte) (*state.ChainHeader, error) {
	kb, err := GetByChainId(chainId)
	header := state.ChainHeader{}
	err = UnmarshalQuery(kb.Data, &header)
	if err != nil {
		return nil, err
	}
	return &header, nil
}

func resolveKeyBookUrl(chainId []byte) (string, error) {
	kb, err := GetByChainId(chainId)
	book := protocol.KeyBook{}
	err = UnmarshalQuery(kb.Data, &book)
	if err != nil {
		return "", err
	}
	return book.GetChainUrl(), nil
}

func resolveKeyPageUrl(chainId []byte) (string, error) {
	res, err := GetByChainId(chainId)
	if err != nil {
		return "", err
	}
	kp := protocol.KeyPage{}
	err = UnmarshalQuery(res.Data, &kp)
	if err != nil {
		return "", err
	}
	return kp.GetChainUrl(), nil
}

func nonceFromTimeNow() uint64 {
	t := time.Now()
	return uint64(t.Unix()*1e6) + uint64(t.Nanosecond())/1e3
}

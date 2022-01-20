package cmd

import (
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
	res := new(api2.ChainQueryResponse)
	res.Data = rec
	if err := Client.Request(context.Background(), "query", &params, res); err != nil {
		return nil, err
	}
	return res.MainChain, nil
}

func getRecordById(chainId []byte, rec interface{}) (*api2.MerkleState, error) {
	params := api2.ChainIdQuery{
		ChainId: chainId,
	}
	res := new(api2.ChainQueryResponse)
	res.Data = rec
	if err := Client.Request(context.Background(), "query-chain", &params, res); err != nil {
		return nil, err
	}
	return res.MainChain, nil
}

func prepareSigner(origin *url2.URL, args []string) ([]string, *transactions.Header, []byte, error) {
	var privKey []byte
	var err error

	ct := 0
	if len(args) == 0 {
		return nil, nil, nil, fmt.Errorf("insufficent arguments on comand line")
	}

	hdr := transactions.Header{}
	hdr.Origin = origin
	hdr.KeyPageHeight = 1
	hdr.KeyPageIndex = 0

	if IsLiteAccount(origin.String()) == true {
		privKey, err = LookupByLabel(origin.String())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to find private key for lite account %s %v", origin.String(), err)
		}
		return args, &hdr, privKey, nil
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
			hdr.KeyPageIndex = uint64(v)
		}
	}

	keyInfo, err := getKey(origin.String(), privKey[32:])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get key for %q : %v", origin, err)
	}

	ms, err := getRecord(keyInfo.KeyPage, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get %q : %v", keyInfo.KeyPage, err)
	}

	hdr.KeyPageIndex = keyInfo.Index
	hdr.KeyPageHeight = ms.Height

	return args[ct:], &hdr, privKey, nil
}

func jsonUnmarshalAccount(data []byte) (state.Chain, error) {
	var typ struct {
		Type types.AccountType
	}
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	account, err := protocol.NewChain(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, account)
	if err != nil {
		return nil, err
	}

	return account, nil
}

func signGenTx(binaryPayload []byte, origin *url2.URL, hdr *transactions.Header, privKey []byte, nonce uint64) (*transactions.ED25519Sig, error) {
	gtx := new(transactions.Envelope)
	gtx.Transaction = new(transactions.Transaction)
	gtx.Transaction.Body = binaryPayload

	hdr.Nonce = nonce
	gtx.Transaction.Header = *hdr

	ed := new(transactions.ED25519Sig)
	err := ed.Sign(nonce, privKey, gtx.Transaction.Hash())
	if err != nil {
		return nil, err
	}
	return ed, nil
}

func prepareGenTxV2(jsonPayload, binaryPayload []byte, origin *url2.URL, si *transactions.Header, privKey []byte, nonce uint64) (*api2.TxRequest, error) {
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
	params.Signer.Nonce = nonce
	params.Origin = origin
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

// Remarshal uses mapstructure to convert a generic JSON-decoded map into a struct.
func Remarshal(src interface{}, dst interface{}) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

// This is a hack to reduce how much we have to change
type QueryResponse struct {
	Type           string                      `json:"type,omitempty"`
	MainChain      *api2.MerkleState           `json:"mainChain,omitempty"`
	Data           interface{}                 `json:"data,omitempty"`
	ChainId        []byte                      `json:"chainId,omitempty"`
	Origin         string                      `json:"origin,omitempty"`
	KeyPage        *api2.KeyPage               `json:"keyPage,omitempty"`
	Txid           []byte                      `json:"txid,omitempty"`
	Signatures     []*transactions.ED25519Sig  `json:"signatures,omitempty"`
	Status         *protocol.TransactionStatus `json:"status,omitempty"`
	SyntheticTxids [][32]byte                  `json:"syntheticTxids,omitempty"`
}

func GetUrl(url string) (*QueryResponse, error) {
	var res QueryResponse

	u, err := url2.Parse(url)
	params := api2.UrlQuery{}
	params.Url = u.String()

	err = queryAs("query", &params, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func queryAs(method string, input, output interface{}) error {
	err := Client.Request(context.Background(), method, input, output)
	if err == nil {
		return nil
	}

	ret, err := PrintJsonRpcError(err)
	if err != nil {
		return err
	}

	return fmt.Errorf("%v", ret)
}

func dispatchTxRequest(action string, payload encoding.BinaryMarshaler, origin *url2.URL, si *transactions.Header, privKey []byte) (*api2.TxResponse, error) {
	dataBinary, err := payload.MarshalBinary()
	if err != nil {
		return nil, err
	}

	var data []byte
	if action == "execute" {
		data, err = json.Marshal(hex.EncodeToString(dataBinary))
	} else {
		data, err = json.Marshal(payload)
	}
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
	Result    interface{}   `json:"result"`
}

type ActionDataResponse struct {
	EntryHash types.Bytes32 `json:"entryHash"`
	ActionResponse
}

type ActionLiteDataResponse struct {
	AccountUrl types.String  `json:"accountUrl"`
	AccountId  types.Bytes32 `json:"accountId"`
	ActionDataResponse
}

func ActionResponseFromLiteData(r *api2.TxResponse, accountUrl string, accountId []byte, entryHash []byte) *ActionLiteDataResponse {
	ar := &ActionLiteDataResponse{}
	ar.AccountUrl = types.String(accountUrl)
	ar.AccountId.FromBytes(accountId)
	ar.ActionDataResponse = *ActionResponseFromData(r, entryHash)
	return ar
}

func (a *ActionLiteDataResponse) Print() (string, error) {
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
		s, err := a.ActionDataResponse.Print()
		if err != nil {
			return "", err
		}
		out = fmt.Sprintf("\n\tAccount Url\t\t:%s\n", a.AccountUrl[:])
		out += fmt.Sprintf("\n\tAccount Id\t\t:%x\n", a.AccountId[:])
		out += s[1:]
	}
	return out, nil
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
	ApiToString = map[types.AccountType]string{
		types.AccountTypeLiteTokenAccount: "Lite Account",
		types.AccountTypeTokenAccount:     "ADI Token Account",
		types.AccountTypeIdentity:         "ADI",
		types.AccountTypeKeyBook:          "Key Book",
		types.AccountTypeKeyPage:          "Key Page",
		types.AccountTypeDataAccount:      "Data Chain",
		types.AccountTypeLiteDataAccount:  "Lite Data Chain",
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

func printGeneralTransactionParameters(res *api2.TransactionQueryResponse) string {
	out := fmt.Sprintf("---\n")
	out += fmt.Sprintf("  - Transaction           : %x\n", res.Txid)
	out += fmt.Sprintf("  - Signer Url            : %s\n", res.Origin)
	out += fmt.Sprintf("  - Signatures            :\n")
	for _, sig := range res.Signatures {
		out += fmt.Sprintf("  -                       : %x (sig) / %x (key)\n", sig.Signature, sig.PublicKey)
	}
	out += fmt.Sprintf("  - Key Page              : %d (height) / %d (index)\n", res.KeyPage.Height, res.KeyPage.Index)
	out += fmt.Sprintf("===\n")
	return out
}

func PrintJson(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func PrintChainQueryResponseV2(res *QueryResponse) (string, error) {
	if WantJsonOutput || res.Type == "dataEntry" {
		return PrintJson(res)
	}

	out, err := outputForHumans(res)
	if err != nil {
		return "", err
	}

	for i, txid := range res.SyntheticTxids {
		out += fmt.Sprintf("  - Synthetic Transaction %d : %x\n", i, txid)
	}
	return out, nil
}

func PrintTransactionQueryResponseV2(res *api2.TransactionQueryResponse) (string, error) {
	if WantJsonOutput {
		return PrintJson(res)
	}

	out, err := outputForHumansTx(res)
	if err != nil {
		return "", err
	}

	for i, txid := range res.SyntheticTxids {
		out += fmt.Sprintf("  - Synthetic Transaction %d : %x\n", i, txid)
	}
	return out, nil
}

func PrintMultiResponse(res *api2.MultiResponse) (string, error) {
	if WantJsonOutput || res.Type == "dataSet" {
		return PrintJson(res)
	}

	var out string
	switch res.Type {
	case "directory":
		out += fmt.Sprintf("\n\tADI Entries: start = %d, count = %d, total = %d\n", res.Start, res.Count, res.Total)

		if len(res.OtherItems) == 0 {
			for _, s := range res.Items {
				out += fmt.Sprintf("\t%v\n", s)
			}
			return out, nil
		}

		for _, s := range res.OtherItems {
			qr := new(api2.ChainQueryResponse)
			header := new(state.ChainHeader)
			qr.Data = header
			err := Remarshal(s, qr)
			if err != nil {
				return "", err
			}

			chainDesc := header.Type.Name()
			if err == nil {
				if v, ok := ApiToString[header.Type]; ok {
					chainDesc = v
				}
			}
			out += fmt.Sprintf("\t%v (%s)\n", header.ChainUrl, chainDesc)
		}

	case "txHistory":
		out += fmt.Sprintf("\n\tTrasaction History Start: %d\t Count: %d\t Total: %d\n", res.Start, res.Count, res.Total)
		for i := range res.Items {
			// Convert the item to a transaction query response
			txr := new(api2.TransactionQueryResponse)
			err := Remarshal(res.Items[i], txr)
			if err != nil {
				return "", err
			}

			s, err := PrintTransactionQueryResponseV2(txr)
			if err != nil {
				return "", err
			}
			out += s
		}
	}

	return out, nil
}

func outputForHumans(res *QueryResponse) (string, error) {
	switch string(res.Type) {
	case types.AccountTypeLiteTokenAccount.String():
		ata := protocol.LiteTokenAccount{}
		err := Remarshal(res.Data, &ata)
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
	case types.AccountTypeTokenAccount.String():
		ata := protocol.TokenAccount{}
		err := Remarshal(res.Data, &ata)
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
	case types.AccountTypeIdentity.String():
		adi := protocol.ADI{}
		err := Remarshal(res.Data, &adi)
		if err != nil {
			return "", err
		}

		var out string
		out += fmt.Sprintf("\n\tADI url\t\t:\t%v\n", adi.ChainUrl)
		out += fmt.Sprintf("\tKey Book url\t:\t%s\n", adi.KeyBook)

		return out, nil
	case types.AccountTypeKeyBook.String():
		book := protocol.KeyBook{}
		err := Remarshal(res.Data, &book)
		if err != nil {
			return "", err
		}

		var out string
		out += fmt.Sprintf("\n\tPage Index\t\tKey Page Url\n")
		for i, v := range book.Pages {
			out += fmt.Sprintf("\t%d\t\t:\t%s\n", i, v)
		}
		return out, nil
	case types.AccountTypeKeyPage.String():
		ss := protocol.KeyPage{}
		err := Remarshal(res.Data, &ss)
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
	default:
		data, err := json.Marshal(res.Data)
		if err != nil {
			return "", err
		}
		out := fmt.Sprintf("Unknown account type %s:\n\t%s\n", res.Type, data)
		return out, nil
	}
}

func outputForHumansTx(res *api2.TransactionQueryResponse) (string, error) {
	switch string(res.Type) {
	case types.TxTypeSendTokens.String():
		tx := response.TokenTx{}
		err := Remarshal(res.Data, &tx)
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
		err := Remarshal(res.Data, &deposit)
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
	case types.TxTypeSyntheticCreateChain.String():
		scc := new(protocol.SyntheticCreateChain)
		err := Remarshal(res.Data, &scc)
		if err != nil {
			return "", err
		}

		var out string
		for _, cp := range scc.Chains {
			c, err := protocol.UnmarshalChain(cp.Data)
			if err != nil {
				return "", err
			}
			// unmarshal
			verb := "Created"
			if cp.IsUpdate {
				verb = "Updated"
			}
			out += fmt.Sprintf("%s %v (%v)\n", verb, c.Header().ChainUrl, c.Header().Type)
		}
		return out, nil
	case types.TxTypeCreateIdentity.String():
		id := protocol.CreateIdentity{}
		err := Remarshal(res.Data, &id)
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
		data, err := json.Marshal(res.Data)
		if err != nil {
			return "", err
		}
		out := fmt.Sprintf("Unknown transaction type %s:\n\t%s\n", res.Type, data)
		return out, nil
	}
}

func getChainHeaderFromChainId(chainId []byte) (*state.ChainHeader, error) {
	kb, err := GetByChainId(chainId)
	header := state.ChainHeader{}
	err = Remarshal(kb.Data, &header)
	if err != nil {
		return nil, err
	}
	return &header, nil
}

func resolveKeyBookUrl(chainId []byte) (string, error) {
	kb, err := GetByChainId(chainId)
	book := protocol.KeyBook{}
	err = Remarshal(kb.Data, &book)
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
	err = Remarshal(res.Data, &kp)
	if err != nil {
		return "", err
	}
	return kp.GetChainUrl(), nil
}

func nonceFromTimeNow() uint64 {
	t := time.Now()
	return uint64(t.Unix()*1e6) + uint64(t.Nanosecond())/1e3
}

package cmd

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	api2 "gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	url2 "gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

func runCmdFunc(fn func([]string) (string, error)) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		out, err := fn(args)
		printOutput(cmd, out, err)
	}
}

func getRecord(urlStr string, rec interface{}) (*api2.MerkleState, error) {
	u, err := url2.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	params := api2.UrlQuery{
		Url: u,
	}
	res := new(api2.ChainQueryResponse)
	res.Data = rec
	if err := Client.RequestAPIv2(context.Background(), "query", &params, res); err != nil {
		return nil, err
	}
	return res.MainChain, nil
}

func prepareSigner(origin *url2.URL, args []string) ([]string, *signing.Signer, error) {
	ct := 0
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("insufficent arguments on comand line")
	}

	signer := new(signing.Signer)
	signer.Type = protocol.SignatureTypeLegacyED25519
	signer.Timestamp = nonceFromTimeNow()

	if IsLiteAccount(origin.String()) {
		privKey, err := LookupByLabel(origin.String())
		if err != nil {
			return nil, nil, fmt.Errorf("unable to find private key for lite token account %s %v", origin.String(), err)
		}

		signer.Url = origin
		signer.Height = 1
		signer.PrivateKey = privKey
		return args, signer, nil
	}

	privKey, err := resolvePrivateKey(args[0])
	if err != nil {
		return nil, nil, err
	}
	signer.PrivateKey = privKey
	ct++

	keyInfo, err := getKey(origin.String(), privKey[32:])
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get key for %q : %v", origin, err)
	}

	if len(args) < 2 {
		signer.Url = keyInfo.KeyPage
	} else if v, err := strconv.ParseUint(args[1], 10, 64); err == nil {
		signer.Url = protocol.FormatKeyPageUrl(keyInfo.KeyBook, v)
		ct++
	} else {
		signer.Url = keyInfo.KeyPage
	}

	ms, err := getRecord(signer.Url.String(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get %q : %v", keyInfo.KeyPage, err)
	}
	signer.Height = ms.Height

	return args[ct:], signer, nil
}

func parseArgsAndPrepareSigner(args []string) ([]string, *url2.URL, *signing.Signer, error) {
	principal, err := url2.Parse(args[0])
	if err != nil {
		return nil, nil, nil, err
	}

	args, signer, err := prepareSigner(principal, args[1:])
	if err != nil {
		return nil, nil, nil, err
	}

	return args, principal, signer, nil
}

func IsLiteAccount(url string) bool {
	u, err := url2.Parse(url)
	if err != nil {
		log.Fatal(err)
	}
	key, _, _ := protocol.ParseLiteTokenAddress(u)
	return key != nil
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
	Signatures     []protocol.Signature        `json:"signatures,omitempty"`
	Status         *protocol.TransactionStatus `json:"status,omitempty"`
	SyntheticTxids [][32]byte                  `json:"syntheticTxids,omitempty"`
}

func GetUrl(url string) (*QueryResponse, error) {
	var res QueryResponse

	u, err := url2.Parse(url)
	if err != nil {
		return nil, err
	}
	params := api2.UrlQuery{}
	params.Url = u

	err = queryAs("query", &params, &res)
	if err != nil {
		return nil, err
	}

	return &res, nil
}

func queryAs(method string, input, output interface{}) error {
	err := Client.RequestAPIv2(context.Background(), method, input, output)
	if err == nil {
		return nil
	}

	ret, err := PrintJsonRpcError(err)
	if err != nil {
		return err
	}

	return fmt.Errorf("%v", ret)
}

func dispatchTxAndPrintResponse(action string, payload protocol.TransactionBody, txHash []byte, origin *url2.URL, signer *signing.Signer) (string, error) {
	res, err := dispatchTxRequest(action, payload, txHash, origin, signer)
	if err != nil {
		return "", err
	}

	return ActionResponseFrom(res).Print()
}

func dispatchTxRequest(action string, payload protocol.TransactionBody, txHash []byte, origin *url2.URL, signer *signing.Signer) (*api2.TxResponse, error) {
	var env *protocol.Envelope
	var sig protocol.Signature
	var err error
	switch {
	case payload != nil && txHash == nil:
		env, err = buildEnvelope(payload, origin)
		if err != nil {
			return nil, err
		}
		sig, err = signer.Initiate(env.Transaction)
	case payload == nil && txHash != nil:
		payload = new(protocol.SignPending)
		env = new(protocol.Envelope)
		env.TxHash = txHash
		env.Transaction = new(protocol.Transaction)
		env.Transaction.Body = payload
		env.Transaction.Header.Principal = origin
		sig, err = signer.Sign(txHash)
	default:
		panic("cannot specify a transaction hash and a payload")
	}
	if err != nil {
		return nil, err
	}
	env.Signatures = append(env.Signatures, sig)

	req := new(api2.TxRequest)
	req.TxHash = txHash
	req.Origin = env.Transaction.Header.Principal
	req.Signer.Nonce = sig.GetTimestamp()
	req.Signer.Url = sig.GetSigner()
	req.Signer.PublicKey = sig.GetPublicKey()
	req.KeyPage.Height = sig.GetSignerHeight()
	req.Signature = sig.GetSignature()
	req.Memo = env.Transaction.Header.Memo
	req.Metadata = env.Transaction.Header.Metadata

	if TxPretend {
		req.CheckOnly = true
	}

	if action == "execute" {
		dataBinary, err := payload.MarshalBinary()
		if err != nil {
			return nil, err
		}
		req.Payload = hex.EncodeToString(dataBinary)
	} else {
		req.Payload = payload
	}
	if err != nil {
		return nil, err
	}

	var res api2.TxResponse
	if err := Client.RequestAPIv2(context.Background(), action, req, &res); err != nil {
		_, err := PrintJsonRpcError(err)
		return nil, err
	}

	return &res, nil
}

func buildEnvelope(payload protocol.TransactionBody, origin *url2.URL) (*protocol.Envelope, error) {
	env := new(protocol.Envelope)
	env.Transaction = new(protocol.Transaction)
	env.Transaction.Body = payload
	env.Transaction.Header.Principal = origin
	env.Transaction.Header.Memo = Memo

	if Metadata == "" {
		return env, nil
	}

	if !strings.Contains(Metadata, ":") {
		env.Transaction.Header.Metadata = []byte(Metadata)
		return env, nil
	}

	dataSet := strings.Split(Metadata, ":")
	switch dataSet[0] {
	case "hex":
		bytes, err := hex.DecodeString(dataSet[1])
		if err != nil {
			return nil, err
		}
		env.Transaction.Header.Metadata = bytes
	case "base64":
		bytes, err := base64.RawStdEncoding.DecodeString(dataSet[1])
		if err != nil {
			return nil, err
		}
		env.Transaction.Header.Metadata = bytes
	default:
		env.Transaction.Header.Metadata = []byte(dataSet[1])
	}
	return env, nil
}

type ActionResponse struct {
	TransactionHash types.Bytes  `json:"transactionHash"`
	EnvelopeHash    types.Bytes  `json:"envelopeHash"`
	SimpleHash      types.Bytes  `json:"simpleHash"`
	Log             types.String `json:"log"`
	Code            types.String `json:"code"`
	Codespace       types.String `json:"codespace"`
	Error           types.String `json:"error"`
	Mempool         types.String `json:"mempool"`
	Result          interface{}  `json:"result"`
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
	_ = ar.AccountId.FromBytes(accountId)
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
	_ = ar.EntryHash.FromBytes(entryHash)
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
	ar := &ActionResponse{
		TransactionHash: r.TransactionHash,
		EnvelopeHash:    r.EnvelopeHash,
		SimpleHash:      r.SimpleHash,
		Error:           types.String(r.Message),
		Code:            types.String(fmt.Sprint(r.Code)),
		Result:          r.Result,
	}
	if r.Code != 0 {
		return ar
	}

	result := new(protocol.TransactionStatus)
	if Remarshal(r.Result, result) != nil {
		return ar
	}

	ar.Code = types.String(fmt.Sprint(result.Code))
	ar.Error = types.String(result.Message)
	return ar
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
		out += fmt.Sprintf("\n\tTransaction Hash\t: %x\n", a.TransactionHash)
		out += fmt.Sprintf("\tEnvelope Hash\t\t: %x\n", a.EnvelopeHash)
		out += fmt.Sprintf("\tSimple Hash\t\t: %x\n", a.SimpleHash)
		if !ok {
			out += fmt.Sprintf("\tError code\t\t: %s\n", a.Code)
		} else {
			//nolint:gosimple
			out += fmt.Sprintf("\tError code\t\t: ok\n")
		}
		if a.Error != "" {
			out += fmt.Sprintf("\tError\t\t\t: %s\n", a.Error)
		}
		if a.Log != "" {
			out += fmt.Sprintf("\tLog\t\t\t: %s\n", a.Log)
		}
		if a.Codespace != "" {
			out += fmt.Sprintf("\tCodespace\t\t: %s\n", a.Codespace)
		}
		if a.Result != nil {
			r := a.Result.(map[string]interface{})
			out += "\tResult\t\t\t: "
			if t, ok := r["result"]; ok {
				d, err := json.Marshal(t)
				if err != nil {
					out += fmt.Sprintf("error remarshaling result %v\n", t)
				} else {
					v, err := protocol.UnmarshalTransactionResultJSON(d)
					if err != nil {
						out += fmt.Sprintf("error unmarshaling transaction result %v", err)
					} else {
						out += outputTransactionResultForHumans(v)
					}
				}
			}
		}
	}

	if ok {
		return out, nil
	}
	return "", errors.New(out)
}

func outputTransactionResultForHumans(t protocol.TransactionResult) string {
	var out string

	switch c := t.(type) {
	case *protocol.AddCreditsResult:
		amt, err := formatAmount(protocol.ACME, &c.Amount)
		if err != nil {
			amt = "unknown"
		}
		out += fmt.Sprintf("Oracle\t$%.2f / ACME\n", float64(c.Oracle)/protocol.AcmeOraclePrecision)
		out += fmt.Sprintf("\t\t\t\t  Credits\t%.2f\n", float64(c.Credits)/protocol.CreditPrecision)
		out += fmt.Sprintf("\t\t\t\t  Amount\t%s\n", amt)
	case *protocol.WriteDataResult:
		out += fmt.Sprintf("Account URL\t%s\n", c.AccountUrl)
		out += fmt.Sprintf("\t\t\t\t  Account ID\t%x\n", c.AccountID)
		out += fmt.Sprintf("\t\t\t\t  Entry Hash\t%x\n", c.EntryHash)
	}
	return out
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
	ApiToString = map[protocol.AccountType]string{
		protocol.AccountTypeLiteTokenAccount: "Lite Account",
		protocol.AccountTypeTokenAccount:     "ADI Token Account",
		protocol.AccountTypeIdentity:         "ADI",
		protocol.AccountTypeKeyBook:          "Key Book",
		protocol.AccountTypeKeyPage:          "Key Page",
		protocol.AccountTypeDataAccount:      "Data Chain",
		protocol.AccountTypeLiteDataAccount:  "Lite Data Chain",
	}
)

func amountToBigInt(tokenUrl string, amount string) (*big.Int, error) {
	//query the token
	qr, err := GetUrl(tokenUrl)
	if err != nil {
		return nil, fmt.Errorf("error retrieving token url, %v", err)
	}
	t := protocol.TokenIssuer{}
	err = Remarshal(qr.Data, &t)
	if err != nil {
		return nil, err
	}

	amt, _ := big.NewFloat(0).SetPrec(128).SetString(amount)
	if amt == nil {
		return nil, fmt.Errorf("invalid amount %s", amount)
	}
	oneToken := big.NewFloat(math.Pow(10.0, float64(t.Precision)))
	amt.Mul(amt, oneToken)
	iAmt, _ := amt.Int(big.NewInt(0))
	return iAmt, nil
}

func GetTokenUrlFromAccount(u *url2.URL) (*url2.URL, error) {
	var err error
	var tokenUrl *url2.URL
	if IsLiteAccount(u.String()) {
		_, tokenUrl, err = protocol.ParseLiteTokenAddress(u)
		if err != nil {
			return nil, fmt.Errorf("cannot extract token url from lite token account, %v", err)
		}
	} else {
		res, err := GetUrl(u.String())
		if err != nil {
			return nil, err
		}
		if res.Type != protocol.AccountTypeTokenAccount.String() {
			return nil, fmt.Errorf("expecting token account but received %s", res.Type)
		}
		ta := protocol.TokenAccount{}
		err = Remarshal(res.Data, &ta)
		if err != nil {
			return nil, fmt.Errorf("error remarshaling token account, %v", err)
		}
		tokenUrl = ta.TokenUrl
	}
	if tokenUrl == nil {
		return nil, fmt.Errorf("invalid token url was obtained from %s", u.String())
	}
	return tokenUrl, nil
}
func amountToString(precision uint64, amount *big.Int) string {
	bf := big.Float{}
	bd := big.Float{}
	bd.SetFloat64(math.Pow(10.0, float64(precision)))
	bf.SetInt(amount)
	bal := big.Float{}
	bal.Quo(&bf, &bd)
	return bal.Text('f', int(precision))
}

func formatAmount(tokenUrl string, amount *big.Int) (string, error) {
	//query the token
	tokenData, err := GetUrl(tokenUrl)
	if err != nil {
		return "", fmt.Errorf("error retrieving token url, %v", err)
	}
	t := protocol.TokenIssuer{}
	err = Remarshal(tokenData.Data, &t)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s %s", amountToString(t.Precision, amount), t.Symbol), nil
}

//nolint:gosimple
func printGeneralTransactionParameters(res *api2.TransactionQueryResponse) string {
	out := fmt.Sprintf("---\n")
	out += fmt.Sprintf("  - Transaction           : %x\n", res.TransactionHash)
	out += fmt.Sprintf("  - Signer Url            : %s\n", res.Origin)
	out += fmt.Sprintf("  - Signatures            :\n")
	for _, sig := range res.Signatures {
		out += fmt.Sprintf("  -                       : %s / %x (sig) / %x (key)\n", sig.GetSigner(), sig.GetSignature(), sig.GetPublicKey())
	}
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

	for _, receipt := range res.Receipts {
		// // TODO Figure out how to include the directory receipt and block
		// out += fmt.Sprintf("Receipt from %v#chain/%s in block %d\n", receipt.Account, receipt.Chain, receipt.DirectoryBlock)
		out += fmt.Sprintf("Receipt from %v#chain/%s\n", receipt.Account, receipt.Chain)
		if receipt.Error != "" {
			out += fmt.Sprintf("  Error!! %s\n", receipt.Error)
		}
		if !receipt.Receipt.Convert().Validate() {
			//nolint:gosimple
			out += fmt.Sprintf("  Invalid!!\n")
		}
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
			var data json.RawMessage
			qr.Data = &data
			err := Remarshal(s, qr)
			if err != nil {
				return "", err
			}

			account, err := protocol.UnmarshalAccountJSON(data)
			if err != nil {
				return "", err
			}

			chainDesc := account.GetType().String()
			if err == nil {
				if v, ok := ApiToString[account.GetType()]; ok {
					chainDesc = v
				}
			}
			out += fmt.Sprintf("\t%v (%s)\n", account.Header().Url, chainDesc)
		}
	case "pending":
		out += fmt.Sprintf("\n\tPending Tranactions -> Start: %d\t Count: %d\t Total: %d\n", res.Start, res.Count, res.Total)
		for i, item := range res.Items {
			out += fmt.Sprintf("\t%d\t%s", i, item)
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

//nolint:gosimple
func outputForHumans(res *QueryResponse) (string, error) {
	switch string(res.Type) {
	case protocol.AccountTypeLiteTokenAccount.String():
		ata := protocol.LiteTokenAccount{}
		err := Remarshal(res.Data, &ata)
		if err != nil {
			return "", err
		}

		amt, err := formatAmount(ata.TokenUrl.String(), &ata.Balance)
		if err != nil {
			amt = "unknown"
		}

		var out string
		out += fmt.Sprintf("\n\tAccount Url\t:\t%v\n", ata.Url)
		out += fmt.Sprintf("\tToken Url\t:\t%v\n", ata.TokenUrl)
		out += fmt.Sprintf("\tBalance\t\t:\t%s\n", amt)
		out += fmt.Sprintf("\tCredits\t\t:\t%d\n", protocol.CreditPrecision*ata.CreditBalance)
		out += fmt.Sprintf("\tNonce\t\t:\t%d\n", ata.Nonce)

		return out, nil
	case protocol.AccountTypeTokenAccount.String():
		ata := protocol.TokenAccount{}
		err := Remarshal(res.Data, &ata)
		if err != nil {
			return "", err
		}

		amt, err := formatAmount(ata.TokenUrl.String(), &ata.Balance)
		if err != nil {
			amt = "unknown"
		}

		var out string
		out += fmt.Sprintf("\n\tAccount Url\t:\t%v\n", ata.Url)
		out += fmt.Sprintf("\tToken Url\t:\t%s\n", ata.TokenUrl)
		out += fmt.Sprintf("\tBalance\t\t:\t%s\n", amt)
		out += fmt.Sprintf("\tKey Book Url\t:\t%s\n", ata.KeyBook)

		return out, nil
	case protocol.AccountTypeIdentity.String():
		adi := protocol.ADI{}
		err := Remarshal(res.Data, &adi)
		if err != nil {
			return "", err
		}

		var out string
		out += fmt.Sprintf("\n\tADI url\t\t:\t%v\n", adi.Url)
		out += fmt.Sprintf("\tKey Book url\t:\t%s\n", adi.KeyBook)

		return out, nil
	case protocol.AccountTypeKeyBook.String():
		book := protocol.KeyBook{}
		err := Remarshal(res.Data, &book)
		if err != nil {
			return "", err
		}

		var out string
		out += fmt.Sprintf("\n\tPage Count\n")
		out += fmt.Sprintf("\t%d\n", book.PageCount)
		return out, nil
	case protocol.AccountTypeKeyPage.String():
		ss := protocol.KeyPage{}
		err := Remarshal(res.Data, &ss)
		if err != nil {
			return "", err
		}

		out := fmt.Sprintf("\n\tCredit Balance\t:\t%d\n", protocol.CreditPrecision*ss.CreditBalance)
		out += fmt.Sprintf("\n\tIndex\tNonce\tPublic Key\t\t\t\t\t\t\t\tKey Name\n")
		for i, k := range ss.Keys {
			keyName := ""
			name, err := FindLabelFromPubKey(k.PublicKey)
			if err == nil {
				keyName = name
			}
			out += fmt.Sprintf("\t%d\t%d\t%x\t%s", i, k.Nonce, k.PublicKey, keyName)
		}
		return out, nil
	case "token", protocol.AccountTypeTokenIssuer.String():
		ti := protocol.TokenIssuer{}
		err := Remarshal(res.Data, &ti)
		if err != nil {
			return "", err
		}

		out := fmt.Sprintf("\n\tToken URL\t:\t%s", ti.Url)
		out += fmt.Sprintf("\n\tSymbol\t\t:\t%s", ti.Symbol)
		out += fmt.Sprintf("\n\tPrecision\t:\t%d", ti.Precision)
		if ti.SupplyLimit != nil {
			out += fmt.Sprintf("\n\tSupply Limit\t\t:\t%s", amountToString(ti.Precision, ti.SupplyLimit))
		}
		out += fmt.Sprintf("\n\tTokens Issued\t:\t%s", amountToString(ti.Precision, &ti.Issued))
		out += fmt.Sprintf("\n\tProperties URL\t:\t%s", ti.Properties)
		out += "\n"
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
	case protocol.TransactionTypeSendTokens.String():
		tx := new(api.TokenSend)
		err := Remarshal(res.Data, &tx)
		if err != nil {
			return "", err
		}

		var out string
		for i := range tx.To {
			amt, err := formatAmount("acc://ACME", &tx.To[i].Amount)
			if err != nil {
				amt = "unknown"
			}
			out += fmt.Sprintf("Send %s from %s to %s\n", amt, res.Origin, tx.To[i].Url)
			out += fmt.Sprintf("  - Synthetic Transaction : %x\n", tx.To[i].Txid)
		}

		out += printGeneralTransactionParameters(res)
		return out, nil
	case protocol.TransactionTypeSyntheticDepositTokens.String():
		deposit := new(protocol.SyntheticDepositTokens)
		err := Remarshal(res.Data, &deposit)
		if err != nil {
			return "", err
		}

		out := "\n"
		amt, err := formatAmount(deposit.Token.String(), &deposit.Amount)
		if err != nil {
			amt = "unknown"
		}
		out += fmt.Sprintf("Receive %s to %s (cause: %X)\n", amt, res.Origin, deposit.Cause)

		out += printGeneralTransactionParameters(res)
		return out, nil
	case protocol.TransactionTypeSyntheticCreateChain.String():
		scc := new(protocol.SyntheticCreateChain)
		err := Remarshal(res.Data, &scc)
		if err != nil {
			return "", err
		}

		var out string
		for _, cp := range scc.Chains {
			c, err := protocol.UnmarshalAccount(cp.Data)
			if err != nil {
				return "", err
			}
			// unmarshal
			verb := "Created"
			if cp.IsUpdate {
				verb = "Updated"
			}
			out += fmt.Sprintf("%s %v (%v)\n", verb, c.Header().Url, c.GetType())
		}
		return out, nil
	case protocol.TransactionTypeCreateIdentity.String():
		id := protocol.CreateIdentity{}
		err := Remarshal(res.Data, &id)
		if err != nil {
			return "", err
		}

		out := "\n"
		out += fmt.Sprintf("ADI URL \t\t:\t%s\n", id.Url)
		out += fmt.Sprintf("Key Book URL\t\t:\t%s\n", id.KeyBookUrl)

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

func nonceFromTimeNow() uint64 {
	t := time.Now()
	return uint64(t.Unix()*1e6) + uint64(t.Nanosecond())/1e3
}

func QueryAcmeOracle() (*protocol.AcmeOracle, error) {
	params := api.DataEntryQuery{}
	params.Url = protocol.PriceOracle()

	res := new(api.ChainQueryResponse)
	entry := new(api.DataEntryQueryResponse)
	res.Data = entry

	err := Client.RequestAPIv2(context.Background(), "query-data", &params, &res)
	if err != nil {
		return nil, err
	}

	if entry.Entry.Data == nil {
		return nil, fmt.Errorf("no data in oracle account")
	}
	acmeOracle := new(protocol.AcmeOracle)
	if err = json.Unmarshal(entry.Entry.Data[0], acmeOracle); err != nil {
		return nil, err
	}
	return acmeOracle, err
}

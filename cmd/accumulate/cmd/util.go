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
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"

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

func runTxnCmdFunc(fn func(*url2.URL, *signing.Builder, []string) (string, error)) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		principal, err := url2.Parse(args[0])
		if err != nil {
			printOutput(cmd, "", err)
			return
		}

		args, signer, err := prepareSigner(principal, args[1:])
		if err != nil {
			printOutput(cmd, "", err)
			return
		}

		out, err := fn(principal, signer, args)
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

func prepareSigner(origin *url2.URL, args []string) ([]string, *signing.Builder, error) {
	ct := 0
	if len(args) == 0 {
		return nil, nil, fmt.Errorf("insufficent arguments on comand line")
	}

	signer := new(signing.Builder)
	signer.Type = protocol.SignatureTypeLegacyED25519
	signer.Timestamp = nonceFromTimeNow()

	if IsLiteAccount(origin.String()) {
		privKey, err := LookupByLite(origin.String())
		if err != nil {
			return nil, nil, fmt.Errorf("unable to find private key for lite token account %s %v", origin.String(), err)
		}
		sigType, _, err := resolveKeyTypeAndHash(privKey[32:])
		if err != nil {
			return nil, nil, err
		}
		signer.Type = sigType
		signer.Url = origin
		signer.Version = 1
		signer.SetPrivateKey(privKey)
		return args, signer, nil
	}

	var keyName string
	keyHolder, err := url2.Parse(args[0])
	if err == nil && keyHolder.UserInfo != "" {
		keyName = keyHolder.UserInfo
		keyHolder.UserInfo = ""
	} else {
		keyHolder = origin
		keyName = args[0]
	}

	privKey, err := resolvePrivateKey(keyName)
	if err != nil {
		return nil, nil, err
	}
	signer.SetPrivateKey(privKey)
	ct++

	sigType, keyHash, err := resolveKeyTypeAndHash(privKey[32:])
	if err != nil {
		return nil, nil, err
	}
	signer.Type = sigType

	keyInfo, err := getKey(keyHolder.String(), keyHash)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get key for %q : %v", origin, err)
	}

	if len(args) < 2 {
		signer.Url = keyInfo.Signer
	} else if v, err := strconv.ParseUint(args[1], 10, 64); err == nil {
		signer.Url = protocol.FormatKeyPageUrl(keyInfo.Authority, v)
		ct++
	} else {
		signer.Url = keyInfo.Signer
	}

	var page *protocol.KeyPage
	_, err = getRecord(signer.Url.String(), &page)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get %q : %v", keyInfo.Signer, err)
	}
	signer.Version = page.Version

	return args[ct:], signer, nil
}

func parseArgsAndPrepareSigner(args []string) ([]string, *url2.URL, *signing.Builder, error) {
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

func getAccount(url string) (protocol.Account, error) {
	qr, err := GetUrl(url)
	if err != nil {
		return nil, err
	}

	json, err := json.Marshal(qr.Data)
	if err != nil {
		return nil, err
	}

	return protocol.UnmarshalAccountJSON(json)
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

func dispatchTxAndPrintResponse(action string, payload protocol.TransactionBody, txHash []byte, origin *url2.URL, signer *signing.Builder) (string, error) {
	res, err := dispatchTxRequest(action, payload, txHash, origin, signer)
	if err != nil {
		return "", err
	}

	if !TxNoWait && TxWait > 0 {
		_, err := waitForTxn(res.TransactionHash, TxWait)
		if err != nil {
			var rpcErr jsonrpc2.Error
			if errors.As(err, &rpcErr) {
				return PrintJsonRpcError(err)
			}
			return "", err
		}
	}

	return ActionResponseFrom(res).Print()
}

func dispatchTxRequest(action string, payload protocol.TransactionBody, txHash []byte, origin *url2.URL, signer *signing.Builder) (*api2.TxResponse, error) {
	var env *protocol.Envelope
	var sig protocol.Signature
	var err error
	switch {
	case payload != nil && txHash == nil:
		env, err = buildEnvelope(payload, origin)
		if err != nil {
			return nil, err
		}
		sig, err = signer.Initiate(env.Transaction[0])
		txHash = env.Transaction[0].GetHash()
	case payload == nil && txHash != nil:
		body := new(protocol.RemoteTransaction)
		body.Hash = *(*[32]byte)(txHash)
		payload = body
		txn := new(protocol.Transaction)
		txn.Body = body
		txn.Header.Principal = origin
		env = new(protocol.Envelope)
		env.TxHash = txHash
		env.Transaction = []*protocol.Transaction{txn}
		sig, err = signer.Sign(txHash)
	default:
		panic("cannot specify a transaction hash and a payload")
	}
	if err != nil {
		return nil, err
	}
	env.Signatures = append(env.Signatures, sig)

	keySig := sig.(protocol.KeySignature)

	req := new(api2.TxRequest)
	req.TxHash = txHash
	req.Origin = env.Transaction[0].Header.Principal
	req.Signer.Timestamp = sig.GetTimestamp()
	req.Signer.Url = sig.GetSigner()
	req.Signer.PublicKey = keySig.GetPublicKey()
	req.Signer.SignatureType = sig.Type()
	req.KeyPage.Version = sig.GetSignerVersion()
	req.Signature = sig.GetSignature()
	req.Memo = env.Transaction[0].Header.Memo
	req.Metadata = env.Transaction[0].Header.Metadata

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
	txn := new(protocol.Transaction)
	txn.Body = payload
	txn.Header.Principal = origin
	txn.Header.Memo = Memo
	env := new(protocol.Envelope)
	env.Transaction = []*protocol.Transaction{txn}

	if Metadata == "" {
		return env, nil
	}

	if !strings.Contains(Metadata, ":") {
		txn.Header.Metadata = []byte(Metadata)
		return env, nil
	}

	dataSet := strings.Split(Metadata, ":")
	switch dataSet[0] {
	case "hex":
		bytes, err := hex.DecodeString(dataSet[1])
		if err != nil {
			return nil, err
		}
		txn.Header.Metadata = bytes
	case "base64":
		bytes, err := base64.RawStdEncoding.DecodeString(dataSet[1])
		if err != nil {
			return nil, err
		}
		txn.Header.Metadata = bytes
	default:
		txn.Header.Metadata = []byte(dataSet[1])
	}
	return env, nil
}

type ActionResponse struct {
	TransactionHash types.Bytes                 `json:"transactionHash"`
	SignatureHashes []types.Bytes               `json:"signatureHashes"`
	SimpleHash      types.Bytes                 `json:"simpleHash"`
	Log             types.String                `json:"log"`
	Code            types.String                `json:"code"`
	Codespace       types.String                `json:"codespace"`
	Error           types.String                `json:"error"`
	Mempool         types.String                `json:"mempool"`
	Result          *protocol.TransactionStatus `json:"result"`
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
		SignatureHashes: make([]types.Bytes, len(r.SignatureHashes)),
		SimpleHash:      r.SimpleHash,
		Error:           types.String(r.Message),
		Code:            types.String(fmt.Sprint(r.Code)),
	}
	for i, hash := range r.SignatureHashes {
		ar.SignatureHashes[i] = hash
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
		for i, hash := range a.SignatureHashes {
			out += fmt.Sprintf("\tSignature %d Hash\t: %x\n", i, hash)
		}
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
			out += "\tResult\t\t\t: "
			d, err := json.Marshal(a.Result.Result)
			if err != nil {
				out += fmt.Sprintf("error remarshaling result %v\n", a.Result.Result)
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
		if WantJsonOutput {
			cmd.PrintErrf("{\"error\":%v}\n", err)
		} else {
			cmd.PrintErrf("Error: %v\n", err)
		}
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
	for _, book := range res.SignatureBooks {
		for _, page := range book.Pages {
			out += fmt.Sprintf("  - Signatures            :\n")
			out += fmt.Sprintf("    - Signer              : %s (%v)\n", page.Signer.Url, page.Signer.Type)
			for _, sig := range page.Signatures {
				if sig.Type().IsSystem() {
					out += fmt.Sprintf("      -                   : %v\n", sig.Type())
				} else {
					out += fmt.Sprintf("      -                   : %x (sig) / %x (key)\n", sig.GetSignature(), sig.GetPublicKeyHash())
				}
			}
		}
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

			chainDesc := account.Type().String()
			if err == nil {
				if v, ok := ApiToString[account.Type()]; ok {
					chainDesc = v
				}
			}
			out += fmt.Sprintf("\t%v (%s)\n", account.GetUrl(), chainDesc)
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

		out += fmt.Sprintf("\tCredits\t\t:\t%v\n", protocol.FormatAmount(ata.CreditBalance, protocol.CreditPrecisionPower))
		out += fmt.Sprintf("\tLast Used On\t:\t%v\n", time.Unix(0, int64(ata.LastUsedOn*uint64(time.Microsecond))))
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
		for _, a := range ata.Authorities {
			out += fmt.Sprintf("\tKey Book Url\t:\t%s\n", a.Url)
		}

		return out, nil
	case protocol.AccountTypeIdentity.String():
		adi := protocol.ADI{}
		err := Remarshal(res.Data, &adi)
		if err != nil {
			return "", err
		}

		var out string
		out += fmt.Sprintf("\n\tADI url\t\t:\t%v\n", adi.Url)
		for _, a := range adi.Authorities {
			out += fmt.Sprintf("\tKey Book Url\t:\t%s\n", a.Url)
		}

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

		out := fmt.Sprintf("\n\tCredit Balance\t:\t%v\n", protocol.FormatAmount(ss.CreditBalance, protocol.CreditPrecisionPower))
		out += fmt.Sprintf("\n\tIndex\tNonce\t\tPublic Key\t\t\t\t\t\t\t\tKey Name(s)\n")
		for i, k := range ss.Keys {
			keyName := ""
			name, err := FindLabelFromPublicKeyHash(k.PublicKeyHash)
			if err == nil {
				keyName = name
			}
			out += fmt.Sprintf("\t%d\t%v\t\t%x\t%s\n", i, time.Unix(0, int64(k.LastUsedOn*uint64(time.Microsecond))), k.PublicKeyHash, keyName)
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
	case protocol.AccountTypeLiteIdentity.String():
		li := protocol.LiteIdentity{}
		err := Remarshal(res.Data, &li)
		if err != nil {
			return "", err
		}
		params := api2.DirectoryQuery{}
		params.Url = li.Url
		params.Start = uint64(0)
		params.Count = uint64(10)
		params.Expand = true

		var adiRes api2.MultiResponse
		if err := Client.RequestAPIv2(context.Background(), "query-directory", &params, &adiRes); err != nil {
			ret, err := PrintJsonRpcError(err)
			if err != nil {
				return "", err
			}
			return "", fmt.Errorf("%v", ret)
		}
		return PrintMultiResponse(&adiRes)
	default:
		return printReflection("", "", reflect.ValueOf(res.Data)), nil
	}
}

func outputForHumansTx(res *api2.TransactionQueryResponse) (string, error) {
	typStr := res.Data.(map[string]interface{})["type"].(string)
	typ, ok := protocol.TransactionTypeByName(typStr)
	if !ok {
		return "", fmt.Errorf("Unknown transaction type %s", typStr)
	}

	if typ == protocol.TransactionTypeSendTokens {
		txn := new(api.TokenSend)
		err := Remarshal(res.Data, txn)
		if err != nil {
			return "", err
		}

		tx := txn
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
	}

	txn, err := protocol.NewTransactionBody(typ)
	if err != nil {
		return "", err
	}

	err = Remarshal(res.Data, txn)
	if err != nil {
		return "", err
	}

	switch txn := txn.(type) {
	case *protocol.SyntheticDepositTokens:
		deposit := txn
		out := "\n"
		amt, err := formatAmount(deposit.Token.String(), &deposit.Amount)
		if err != nil {
			amt = "unknown"
		}
		out += fmt.Sprintf("Receive %s to %s (cause: %X)\n", amt, res.Origin, deposit.Cause)

		out += printGeneralTransactionParameters(res)
		return out, nil
	case *protocol.SyntheticCreateChain:
		scc := txn
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
			out += fmt.Sprintf("%s %v (%v)\n", verb, c.GetUrl(), c.Type())
		}
		return out, nil
	case *protocol.CreateIdentity:
		id := txn
		out := "\n"
		out += fmt.Sprintf("ADI URL \t\t:\t%s\n", id.Url)
		out += fmt.Sprintf("Key Book URL\t\t:\t%s\n", id.KeyBookUrl)

		keyName, err := FindLabelFromPublicKeyHash(id.KeyHash)
		if err != nil {
			out += fmt.Sprintf("Public Key \t:\t%x\n", id.KeyHash)
		} else {
			out += fmt.Sprintf("Public Key (name(s)) \t:\t%x (%s)\n", id.KeyHash, keyName)
		}

		out += printGeneralTransactionParameters(res)
		return out, nil

	default:
		return printReflection("", "", reflect.ValueOf(txn)), nil
	}
}

func printReflection(field, indent string, value reflect.Value) string {
	typ := value.Type()
	out := fmt.Sprintf("%s%s:", indent, field)
	if field == "" {
		out = ""
	}

	if typ.AssignableTo(reflect.TypeOf(new(url2.URL))) {
		v := value.Interface().(*url2.URL)
		if v == nil {
			return out + " (nil)\n"
		}
		return out + " " + v.String() + "\n"
	}

	if typ.AssignableTo(reflect.TypeOf(url2.URL{})) {
		v := value.Interface().(url2.URL)
		return out + " " + v.String() + "\n"
	}

	switch value.Kind() {
	case reflect.Ptr, reflect.Interface:
		if value.IsNil() {
			return ""
		}
		return printReflection(field, indent, value.Elem())
	case reflect.Slice, reflect.Array:
		out += "\n"
		for i, n := 0, value.Len(); i < n; i++ {
			out += printReflection(fmt.Sprintf("%d (elem)", i), indent+"   ", value.Index(i))
		}
		return out
	case reflect.Map:
		out += "\n"
		for iter := value.MapRange(); iter.Next(); {
			out += printReflection(fmt.Sprintf("%s (key)", iter.Key()), indent+"   ", iter.Value())
		}
		return out
	case reflect.Struct:
		out += "\n"
		out += fmt.Sprintf("%s   (type): %s\n", indent, natural(typ.Name()))

		callee := value
		m, ok := typ.MethodByName("Type")
		if !ok {
			m, ok = reflect.PtrTo(typ).MethodByName("Type")
			callee = value.Addr()
		}
		if ok && m.Type.NumIn() == 1 && m.Type.NumOut() == 1 {
			v := m.Func.Call([]reflect.Value{callee})[0]
			if _, ok := v.Type().MethodByName("GetEnumValue"); ok {
				out += fmt.Sprintf("%s   %s: %s\n", indent, natural(v.Type().Name()), natural(fmt.Sprint(v)))
			}
		}

		for i, n := 0, value.NumField(); i < n; i++ {
			f := typ.Field(i)
			if !f.IsExported() {
				continue
			}
			out += printReflection(f.Name, indent+"   ", value.Field(i))
		}
		return out
	default:
		return out + " " + fmt.Sprint(value) + "\n"
	}
}

func natural(name string) string {
	var splits []int

	var wasLower bool
	for i, r := range name {
		if wasLower && unicode.IsUpper(r) {
			splits = append(splits, i)
		}
		wasLower = unicode.IsLower(r)
	}

	w := new(strings.Builder)
	w.Grow(len(name) + len(splits))

	var word string
	var split int
	var offset int
	for len(splits) > 0 {
		split, splits = splits[0], splits[1:]
		split -= offset
		offset += split
		word, name = name[:split], name[split:]
		w.WriteString(word)
		w.WriteRune(' ')
	}

	w.WriteString(name)
	return w.String()
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

func ValidateSigType(input string) (protocol.SignatureType, error) {
	var sigtype protocol.SignatureType
	var err error
	input = strings.ToLower(input)
	switch input {
	case "rcd1":
		sigtype = protocol.SignatureTypeRCD1
		err = nil
	case "ed25519":
		sigtype = protocol.SignatureTypeED25519
		err = nil
	case "legacyed25519":
		sigtype = protocol.SignatureTypeLegacyED25519
		err = nil
	default:
		sigtype = protocol.SignatureTypeED25519
		err = nil
	}
	return sigtype, err
}

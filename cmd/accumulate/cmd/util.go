package cmd

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/big"
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

	for _, del := range Delegators {
		u, err := url2.Parse(del)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid delegator %q: %v", del, err)
		}
		signer.AddDelegator(u)
	}

	var privKey []byte
	var err error
	if IsLiteTokenAccount(origin.String()) {
		privKey, err = LookupByLiteTokenUrl(origin.String())
		if err != nil {
			return nil, nil, fmt.Errorf("unable to find private key for lite token account %s %v", origin.String(), err)
		}

	} else if IsLiteIdentity(origin.String()) {
		privKey, err = LookupByLiteIdentityUrl(origin.String())
		if err != nil {
			return nil, nil, fmt.Errorf("unable to find private key for lite identity account %s %v", origin.String(), err)
		}
	}

	if privKey != nil {
		sigType, _, err := resolveKeyTypeAndHash(privKey[32:])
		if err != nil {
			return nil, nil, err
		}
		signer.Type = sigType
		signer.Url = origin.RootIdentity()
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

	privKey, err = resolvePrivateKey(keyName)
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

func IsLiteTokenAccount(url string) bool {
	u, err := url2.Parse(url)
	if err != nil {
		log.Fatal(err)
	}
	key, _, _ := protocol.ParseLiteTokenAddress(u)
	return key != nil
}

func IsLiteIdentity(url string) bool {
	u, err := url2.Parse(url)
	if err != nil {
		log.Fatal(err)
	}
	key, _ := protocol.ParseLiteIdentity(u)
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

func dispatchTxRequest(payload protocol.TransactionBody, txHash []byte, origin *url2.URL, signer *signing.Builder) (*api2.TxResponse, error) {
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
	case payload == nil && txHash != nil:
		body := new(protocol.RemoteTransaction)
		body.Hash = *(*[32]byte)(txHash)
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

	req := new(api2.ExecuteRequest)
	req.Envelope = env
	if TxPretend {
		req.CheckOnly = true
	}

	res, err := Client.ExecuteDirect(context.Background(), req)
	if err != nil {
		_, err := PrintJsonRpcError(err)
		return nil, err
	}

	return res, nil
}

func dispatchTxAndWait(payload protocol.TransactionBody, txHash []byte, origin *url2.URL, signer *signing.Builder) (*api2.TxResponse, error) {
	res, err := dispatchTxRequest(payload, txHash, origin, signer)
	if err != nil {
		return nil, err
	}

	if TxWait == 0 {
		return res, nil
	}

	_, err = waitForTxn(res.TransactionHash, TxWait, TxIgnorePending)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func dispatchTxAndPrintResponse(payload protocol.TransactionBody, txHash []byte, origin *url2.URL, signer *signing.Builder) (string, error) {
	res, err := dispatchTxAndWait(payload, txHash, origin, signer)
	if err != nil {
		return PrintJsonRpcError(err)
	}

	return ActionResponseFrom(res).Print()
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

func ActionResponseFromData(r *api2.TxResponse, entryHash []byte) *ActionDataResponse {
	ar := &ActionDataResponse{}
	_ = ar.EntryHash.FromBytes(entryHash)
	ar.ActionResponse = *ActionResponseFrom(r)
	return ar
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
	ar.Result = result
	return ar
}

type JsonRpcError struct {
	Msg string
	Err jsonrpc2.Error
}

func (e *JsonRpcError) Error() string { return e.Msg }

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
	if IsLiteTokenAccount(u.String()) {
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
	sigtype, ok := protocol.SignatureTypeByName(input)
	if !ok {
		sigtype = protocol.SignatureTypeED25519
	}
	return sigtype, nil
}

func GetAccountStateProof(principal, accountToProve *url2.URL) (proof protocol.AccountStateProof, err error) {
	if principal.LocalTo(accountToProve) {
		return protocol.AccountStateProof{}, nil // Don't need a proof for local accounts
	}

	if accountToProve.Equal(protocol.AcmeUrl()) {
		return protocol.AccountStateProof{}, nil // Don't need a proof for ACME
	}

	// Get a proof of the account state
	req := new(api.GeneralQuery)
	req.Url = accountToProve
	resp := new(api.ChainQueryResponse)
	token := protocol.TokenIssuer{}
	resp.Data = &token

	err = Client.RequestAPIv2(context.Background(), "query", req, resp)
	if err != nil || resp.Type != protocol.AccountTypeTokenIssuer.String() {
		return protocol.AccountStateProof{}, err
	}

	localReceipt := resp.Receipt.Receipt
	//chainid := hex.EncodeToString(localReceipt.Start)
	//chainurl := url2.MustParse(chainid)
	//proof.State = &protocol.TokenIssuer{Url: chainurl}
	proof.State, err = getAccount(accountToProve.String())
	if err != nil {
		return protocol.AccountStateProof{}, err
	}
	// ensure the block is anchored
	timeout := time.After(10 * time.Second)
	ticker := time.Tick(1 * time.Second)
	// Keep trying until we're timed out or get a result/error
	for {
		select {
		// Got a timeout! fail with a timeout error
		case <-timeout:
			return protocol.AccountStateProof{}, nil
		// Got a tick, we should check if the anchor is complete
		case <-ticker:
			// Get a proof of the BVN anchor
			req = new(api.GeneralQuery)
			req.Url = url2.MustParse(fmt.Sprintf("dn/anchors#anchor/%x", localReceipt.Result))
			resp = new(api.ChainQueryResponse)
			err = Client.RequestAPIv2(context.Background(), "query", req, resp)
			if err != nil || resp.Type != protocol.AccountTypeTokenIssuer.String() {
				return protocol.AccountStateProof{}, err
			}
			dirReceipt := resp.Receipt.Receipt
			if dirReceipt.Result != nil {
				return proof, nil
			}
			proof.Receipt = localReceipt.Combine(&dirReceipt)

		}
	}
}

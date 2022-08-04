package cmd

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd"
	"math"
	"math/big"
	"strings"
	"unicode"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

func runCmdFunc(fn func(args []string) (string, error)) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		out, err := fn(args)
		printOutput(cmd, out, err)
	}
}

func runTxnCmdFunc(fn func(principal *url.URL, signers []*signing.Builder, args []string) (string, error)) func(cmd *cobra.Command, args []string) {
	return runCmdFunc(func(args []string) (string, error) {
		principal, err := url.Parse(args[0])
		if err != nil {
			return "", err
		}

		args, signers, err := prepareSigner(principal, args[1:])
		if err != nil {
			return "", err
		}

		return fn(principal, signers, args)
	})
}

func getRecord(urlStr string, rec interface{}) (*api.MerkleState, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	params := api.UrlQuery{
		Url: u,
	}
	res := new(api.ChainQueryResponse)
	res.Data = rec
	if err := Client.RequestAPIv2(context.Background(), "query", &params, res); err != nil {
		return nil, err
	}
	return res.MainChain, nil
}

func prepareSigner(origin *url.URL, args []string) ([]string, []*signing.Builder, error) {
	var signers []*signing.Builder
	for _, name := range AdditionalSigners {
		signer := new(signing.Builder)
		signer.Type = protocol.SignatureTypeLegacyED25519
		err := prepareSignerPage(signer, origin, name)
		if err != nil {
			return nil, nil, err
		}
		signers = append(signers, signer)
	}

	var key *walletd.Key
	var err error
	isLiteTokenAccount, _ := IsLiteTokenAccount(origin.String())
	if isLiteTokenAccount {
		key, err = walletd.LookupByLiteTokenUrl(origin.String())
		if err != nil {
			return nil, nil, fmt.Errorf("unable to find private key for lite token account %s %v", origin.String(), err)
		}

	} else {
		isLiteIdentity, err := IsLiteIdentity(origin.String())
		if err != nil {
			return nil, nil, err
		}
		if isLiteIdentity {
			key, err = walletd.LookupByLiteIdentityUrl(origin.String())
			if err != nil {
				return nil, nil, fmt.Errorf("unable to find private key for lite identity account %s %v", origin.String(), err)
			}
		}
	}

	firstSigner := new(signing.Builder)
	firstSigner.Type = protocol.SignatureTypeLegacyED25519
	firstSigner.SetTimestampToNow()

	for _, del := range Delegators {
		u, err := url.Parse(del)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid delegator %q: %v", del, err)
		}
		firstSigner.AddDelegator(u)
	}

	if key != nil {
		firstSigner.Type = key.KeyInfo.Type
		firstSigner.Url = origin.RootIdentity()
		firstSigner.Version = 1
		firstSigner.SetPrivateKey(key.PrivateKey)
	} else if len(args) > 0 {
		err = prepareSignerPage(firstSigner, origin, args[0])
		if err != nil {
			return nil, nil, err
		}
		args = args[1:]
	} else {
		return nil, nil, fmt.Errorf("key name argument is missing")
	}

	// Put the first signer first
	signers = append(signers, nil)
	copy(signers[1:], signers)
	signers[0] = firstSigner
	return args, signers, nil
}

func prepareSignerPage(signer *signing.Builder, origin *url.URL, signingKey string) error {
	var keyName string
	keyHolder, err := url.Parse(signingKey)
	if err == nil && keyHolder.UserInfo != "" {
		keyName = keyHolder.UserInfo
		keyHolder = keyHolder.WithUserInfo("")
	} else {
		keyHolder = origin
		keyName = signingKey
	}

	key, err := resolvePrivateKey(keyName)
	if err != nil {
		return err
	}
	signer.SetPrivateKey(key.PrivateKey)

	signer.Type = key.KeyInfo.Type

	keyInfo, err := getKey(keyHolder.String(), key.PublicKeyHash())
	if err != nil {
		return fmt.Errorf("failed to get key for %q : %v", origin, err)
	}

	signer.Url = keyInfo.Signer

	var page *protocol.KeyPage
	_, err = getRecord(signer.Url.String(), &page)
	if err != nil {
		return fmt.Errorf("failed to get %q : %v", keyInfo.Signer, err)
	}
	if SignerVersion != 0 {
		signer.Version = uint64(SignerVersion)
	} else {
		signer.Version = page.Version
	}

	return nil
}

func parseArgsAndPrepareSigner(args []string) ([]string, *url.URL, []*signing.Builder, error) {
	principal, err := url.Parse(args[0])
	if err != nil {
		return nil, nil, nil, err
	}

	args, signers, err := prepareSigner(principal, args[1:])
	if err != nil {
		return nil, nil, nil, err
	}

	return args, principal, signers, nil
}

func IsLiteTokenAccount(urlstr string) (bool, error) {
	u, err := url.Parse(strings.Trim(urlstr, " "))
	if err != nil {
		return false, err
	}
	if strings.Contains(u.Hostname(), ".") {
		return false, nil
	}
	key, _, err := protocol.ParseLiteTokenAddress(u)
	if err != nil {
		return false, fmt.Errorf("invalid lite token address : %s", u.String())
	}
	return key != nil, nil
}

func IsLiteIdentity(urlstr string) (bool, error) {
	u, err := url.Parse(strings.Trim(urlstr, " "))
	if err != nil {
		return false, err
	}
	if strings.Contains(u.Hostname(), ".") {
		return false, nil
	}
	key, err := protocol.ParseLiteIdentity(u)
	if err != nil {
		return false, fmt.Errorf("invalid lite identity : %s", u.String())
	}
	return key != nil, nil
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
	MainChain      *api.MerkleState            `json:"mainChain,omitempty"`
	Data           interface{}                 `json:"data,omitempty"`
	ChainId        []byte                      `json:"chainId,omitempty"`
	Origin         string                      `json:"origin,omitempty"`
	KeyPage        *api.KeyPage                `json:"keyPage,omitempty"`
	Txid           []byte                      `json:"txid,omitempty"`
	Signatures     []protocol.Signature        `json:"signatures,omitempty"`
	Status         *protocol.TransactionStatus `json:"status,omitempty"`
	SyntheticTxids [][32]byte                  `json:"syntheticTxids,omitempty"`
}

func GetUrl(urlstr string) (*QueryResponse, error) {
	var res QueryResponse

	u, err := url.Parse(urlstr)
	if err != nil {
		return nil, err
	}
	params := api.UrlQuery{}
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

	_, err = PrintJsonRpcError(err)
	return err
}

func dispatchTxRequest(payload interface{}, origin *url.URL, signers []*signing.Builder) (*api.TxResponse, error) {
	// Convert the payload to an envelope
	var env *protocol.Envelope
	var err error
	switch payload := payload.(type) {
	case *protocol.Envelope:
		env = payload

	case *protocol.Transaction:
		env = new(protocol.Envelope)
		env.Transaction = []*protocol.Transaction{payload}

	case []byte:
		env, err = buildEnvelope(&protocol.RemoteTransaction{
			Hash: *(*[32]byte)(payload),
		}, origin)

	case protocol.TransactionBody:
		env, err = buildEnvelope(payload, origin)

	default:
		panic(fmt.Errorf("%T is not a supported payload type", payload))
	}
	if err != nil {
		return nil, err
	}

	// Resolve the transaction - check on the principal and every signer and delegator
	if remote, ok := env.Transaction[0].Body.(*protocol.RemoteTransaction); ok {
		accounts := []*url.URL{origin}
		for _, signer := range signers {
			accounts = append(accounts, signer.Url)
			accounts = append(accounts, signer.Delegators...)
		}
		var found bool
		for _, account := range accounts {
			req := new(api.GeneralQuery)
			req.Url = account.WithTxID(remote.Hash).AsUrl()
			resp := new(api.TransactionQueryResponse)
			err := queryAs("query", req, resp)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					continue
				}
				return nil, err
			}
			found = true
			env.Transaction[0] = resp.Transaction
			break
		}
		if !found {
			return nil, fmt.Errorf("unable to locate transaction %x", remote.Hash[:4])
		}
	}

	// Sign
	for _, signer := range signers {
		var sig protocol.Signature
		if env.Transaction[0].Header.Initiator == ([32]byte{}) {
			sig, err = signer.Initiate(env.Transaction[0])
		} else {
			sig, err = signer.Sign(env.Transaction[0].GetHash())
		}
		if err != nil {
			return nil, err
		}
		env.Signatures = append(env.Signatures, sig)
	}

	req := new(api.ExecuteRequest)
	req.Envelope = env
	if TxPretend {
		req.CheckOnly = true
	}

	res, err := Client.ExecuteDirect(context.Background(), req)
	if err != nil {
		_, err := PrintJsonRpcError(err)
		return nil, err
	}
	if res.Code != 0 {
		result := new(protocol.TransactionStatus)
		if Remarshal(res.Result, result) != nil {
			return nil, errors.New(errors.StatusEncodingError, res.Message)
		}
		return nil, result.Error
	}

	return res, nil
}

func dispatchTxAndWait(payload interface{}, origin *url.URL, signers []*signing.Builder) (*api.TxResponse, []*api.TransactionQueryResponse, error) {
	res, err := dispatchTxRequest(payload, origin, signers)
	if err != nil {
		return nil, nil, err
	}

	if TxWait == 0 {
		return res, nil, nil
	}

	resps, err := waitForTxn(res.TransactionHash, TxWait, TxIgnorePending)
	if err != nil {
		return nil, nil, err
	}

	return res, resps, nil
}

func dispatchTxAndPrintResponse(payload interface{}, origin *url.URL, signers []*signing.Builder) (string, error) {
	res, resps, err := dispatchTxAndWait(payload, origin, signers)
	if err != nil {
		return PrintJsonRpcError(err)
	}

	result, err := ActionResponseFrom(res).Print()
	if err != nil {
		return "", err
	}
	if res.Code == 0 {
		for _, response := range resps {
			str, err := PrintTransactionQueryResponseV2(response)
			if err != nil {
				return PrintJsonRpcError(err)
			}
			result = fmt.Sprint(result, str, "\n")
		}
	}
	return result, nil
}

func buildEnvelope(payload protocol.TransactionBody, origin *url.URL) (*protocol.Envelope, error) {
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
	TransactionHash types.Bytes                     `json:"transactionHash"`
	SignatureHashes []types.Bytes                   `json:"signatureHashes"`
	SimpleHash      types.Bytes                     `json:"simpleHash"`
	Log             types.String                    `json:"log"`
	Code            types.String                    `json:"code"`
	Codespace       types.String                    `json:"codespace"`
	Error           types.String                    `json:"error"`
	Mempool         types.String                    `json:"mempool"`
	Result          *protocol.TransactionStatus     `json:"result"`
	Flow            []*api.TransactionQueryResponse `json:"flow"`
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

func ActionResponseFromLiteData(r *api.TxResponse, accountUrl string, accountId []byte, entryHash []byte) *ActionLiteDataResponse {
	ar := &ActionLiteDataResponse{}
	ar.AccountUrl = types.String(accountUrl)
	_ = ar.AccountId.FromBytes(accountId)
	ar.ActionDataResponse = *ActionResponseFromData(r, entryHash)
	return ar
}

func ActionResponseFromData(r *api.TxResponse, entryHash []byte) *ActionDataResponse {
	ar := &ActionDataResponse{}
	_ = ar.EntryHash.FromBytes(entryHash)
	ar.ActionResponse = *ActionResponseFrom(r)
	return ar
}

func ActionResponseFrom(r *api.TxResponse) *ActionResponse {
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

	if result.Failed() {
		ar.Code = types.String(fmt.Sprint(result.CodeNum()))
	}
	if result.Error != nil {
		ar.Error = types.String(result.Error.Message)
	}
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

	return parseAmount(amount, t.Precision)
}

func parseAmount(amount string, precision uint64) (*big.Int, error) {
	amt, _ := big.NewFloat(0).SetPrec(128).SetString(amount)
	if amt == nil {
		return nil, fmt.Errorf("invalid amount %s", amount)
	}

	oneToken := big.NewFloat(math.Pow(10.0, float64(precision))) //Convert to fixed point; multiply by the precision
	amt.Mul(amt, oneToken)                                       // Note that we are using floating point here.  Precision can be lost
	round := big.NewFloat(.9)                                    // To adjust for lost precision, round to the nearest int
	if amt.Sign() < 0 {                                          // Just to be safe, account for negative numbers
		round = big.NewFloat(-.9)
	}
	amt.Add(amt, round)               //                              Round up (positive) or down (negative) to the lowest int
	iAmt, _ := amt.Int(big.NewInt(0)) //                              Then convert to a big Int
	return iAmt, nil                  //                              Return the int
}

func GetTokenUrlFromAccount(u *url.URL) (*url.URL, error) {
	var err error
	var tokenUrl *url.URL
	isLiteTokenAccount, err := IsLiteTokenAccount(u.String())
	if err != nil {
		return nil, err
	}
	if isLiteTokenAccount {
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

func QueryAcmeOracle() (*protocol.AcmeOracle, error) {
	resp, err := Client.Describe(context.Background())
	if err != nil {
		return nil, err
	}

	return resp.Values.Oracle, err
}

func ValidateSigType(input string) (protocol.SignatureType, error) {
	sigtype, ok := protocol.SignatureTypeByName(input)
	if !ok {
		sigtype = protocol.SignatureTypeED25519
	}
	return sigtype, nil
}

package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"

	testing2 "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/tools/internal/testdata"
	randPkg "golang.org/x/exp/rand"
)

var keySeed = sha256.Sum256([]byte("test data"))
var key = ed25519.NewKeyFromSeed(keySeed[:])
var rand = randPkg.New(randPkg.NewSource(binary.BigEndian.Uint64(keySeed[:])))

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: gen-testdata <output-file>")
		os.Exit(1)
	}

	file := os.Args[1]

	// If the file is in a directory, make sure the directory exists
	dir := filepath.Dir(file)
	if dir != "" && dir != "." {
		check(os.MkdirAll(dir, 0755))
	}

	ts := &testdata.TestSuite{
		Transactions: txnTests,
		Accounts:     acntTests,
	}

	check(ts.Store(file))
}

type TCG = testdata.TestCaseGroup
type TC = testdata.TestCase

var txnTests = []*TCG{
	{Name: "CreateIdentity", Cases: []*TC{
		txnTest("lite-token-account/ACME", &CreateIdentity{Url: parseUrl("adi"), KeyHash: key[32:]}),
		txnTest("lite-token-account/ACME", &CreateIdentity{Url: parseUrl("adi"), KeyHash: key[32:], KeyBookUrl: parseUrl("adi/book")}),
	}},
	{Name: "CreateTokenAccount", Cases: []*TC{
		txnTest("adi", &CreateTokenAccount{Url: parseUrl("adi/ACME"), TokenUrl: parseUrl("ACME")}),
		txnTest("adi", &CreateTokenAccount{Url: parseUrl("adi/ACME"), TokenUrl: parseUrl("ACME"), Authorities: []*url.URL{parseUrl("adi/book")}}),
		txnTest("adi", &CreateTokenAccount{Url: parseUrl("adi/ACME"), TokenUrl: parseUrl("ACME"), Scratch: true}),
	}},
	{Name: "SendTokens", Cases: []*TC{
		txnTest("adi/ACME", &SendTokens{To: []*TokenRecipient{{Url: parseUrl("other/ACME"), Amount: *new(big.Int).SetInt64(100)}}}),
		txnTest("adi/ACME", &SendTokens{To: []*TokenRecipient{{Url: parseUrl("other/ACME"), Amount: *new(big.Int).SetInt64(100)}}, Meta: json.RawMessage(`{"foo":"bar"}`)}),
		txnTest("adi/ACME", &SendTokens{To: []*TokenRecipient{{Url: parseUrl("alice/ACME"), Amount: *new(big.Int).SetInt64(100)}, {Url: parseUrl("bob/ACME"), Amount: *new(big.Int).SetInt64(100)}}}),
	}},
	{Name: "CreateDataAccount", Cases: []*TC{
		txnTest("adi", &CreateDataAccount{Url: parseUrl("adi/data")}),
	}},
	{Name: "WriteData", Cases: []*TC{
		txnTest("adi", &WriteData{Entry: &AccumulateDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar"), []byte("baz")}}}),
	}},
	{Name: "WriteDataTo", Cases: []*TC{
		txnTest("adi", &WriteDataTo{Recipient: parseUrl("lite-data-account"), Entry: &AccumulateDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar"), []byte("baz")}}}),
	}},
	{Name: "AcmeFaucet", Cases: []*TC{
		txnTest("faucet", &AcmeFaucet{Url: parseUrl("lite-token-account")}),
	}},
	{Name: "CreateToken", Cases: []*TC{
		txnTest("adi", &CreateToken{Url: parseUrl("adi/foocoin"), Symbol: "FOO", Precision: 10}),
	}},
	{Name: "IssueTokens", Cases: []*TC{
		txnTest("adi/foocoin", &IssueTokens{Recipient: parseUrl("adi/foo"), Amount: *new(big.Int).SetInt64(100)}),
	}},
	{Name: "BurnTokens", Cases: []*TC{
		txnTest("adi/foo", &BurnTokens{Amount: *new(big.Int).SetInt64(100)}),
	}},
	{Name: "CreateKeyPage", Cases: []*TC{
		txnTest("adi", &CreateKeyPage{Keys: []*KeySpecParams{{KeyHash: key[32:]}}}),
	}},
	{Name: "CreateKeyBook", Cases: []*TC{
		txnTest("adi", &CreateKeyBook{Url: parseUrl("adi/book"), PublicKeyHash: key[32:]}),
	}},
	{Name: "AddCredits", Cases: []*TC{
		txnTest("lite-token-account", &AddCredits{Recipient: parseUrl("adi/page"), Amount: *big.NewInt(100)}),
	}},
	{Name: "UpdateKeyPage", Cases: []*TC{
		txnTest("adi", &UpdateKeyPage{Operation: []protocol.KeyPageOperation{&AddKeyOperation{Entry: KeySpecParams{KeyHash: key[32:]}}}}),
	}},
	{Name: "SignPending", Cases: []*TC{
		txnTest("adi", &RemoteTransaction{}),
	}},
	{Name: "SyntheticCreateIdentity", Cases: []*TC{
		txnTest("adi", &SyntheticCreateIdentity{SyntheticOrigin: SyntheticOrigin{Cause: [32]byte{1}, Source: testing2.FakeBvn},
			Accounts: []Account{&UnknownAccount{Url: parseUrl("foo")}}}),
	}},
	{Name: "SyntheticWriteData", Cases: []*TC{
		txnTest("adi", &SyntheticWriteData{SyntheticOrigin: SyntheticOrigin{Cause: [32]byte{1}, Source: testing2.FakeBvn},
			Entry: &AccumulateDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar"), []byte("baz")}}}),
	}},
	{Name: "SyntheticDepositTokens", Cases: []*TC{
		txnTest("adi", &SyntheticDepositTokens{SyntheticOrigin: SyntheticOrigin{Cause: [32]byte{1}, Source: testing2.FakeBvn},
			Token: parseUrl("ACME"), Amount: *new(big.Int).SetInt64(10000)}),
	}},
	{Name: "SyntheticDepositCredits", Cases: []*TC{
		txnTest("adi", &SyntheticDepositCredits{SyntheticOrigin: SyntheticOrigin{Cause: [32]byte{1}, Source: testing2.FakeBvn}, Amount: 1234}),
	}},
	{Name: "SyntheticBurnTokens", Cases: []*TC{
		txnTest("adi", &SyntheticBurnTokens{SyntheticOrigin: SyntheticOrigin{Cause: [32]byte{1}, Source: testing2.FakeBvn},
			Amount: *big.NewInt(123456789)}),
	}},
}

var simpleAuth = &AccountAuth{Authorities: []AuthorityEntry{{Url: parseUrl("adi/book")}}}
var managerAuth = &AccountAuth{Authorities: []AuthorityEntry{{Url: parseUrl("adi/book"), Disabled: true}, {Url: parseUrl("adi/mgr")}}}

var acntTests = []*TCG{
	{Name: "Identity", Cases: []*TC{
		testdata.NewAcntTest(&ADI{Url: parseUrl("adi"), AccountAuth: *simpleAuth}),
		testdata.NewAcntTest(&ADI{Url: parseUrl("adi"), AccountAuth: *managerAuth}),
	}},
	{Name: "TokenIssuer", Cases: []*TC{
		testdata.NewAcntTest(&TokenIssuer{Url: parseUrl("adi/foocoin"), AccountAuth: *simpleAuth, Symbol: "FOO", Precision: 10}),
	}},
	{Name: "TokenAccount", Cases: []*TC{
		testdata.NewAcntTest(&TokenAccount{Url: parseUrl("adi/foo"), AccountAuth: *simpleAuth, TokenUrl: parseUrl("adi/foocoin"), Balance: *big.NewInt(123456789)}),
	}},
	{Name: "LiteTokenAccount", Cases: []*TC{
		testdata.NewAcntTest(&LiteTokenAccount{Url: parseUrl("lite-token-account"), TokenUrl: parseUrl("ACME"), Balance: *big.NewInt(12345)}),
	}},
	{Name: "LiteIdentity", Cases: []*TC{
		testdata.NewAcntTest(&LiteIdentity{Url: parseUrl("lite-identity"), LastUsedOn: uint64(rand.Uint32()), CreditBalance: 9835}),
	}},
	{Name: "KeyPage", Cases: []*TC{
		testdata.NewAcntTest(&KeyPage{Url: parseUrl("adi/page"), Keys: []*KeySpec{{PublicKeyHash: key[32:], LastUsedOn: uint64(rand.Uint32()), Delegate: parseUrl("foo/bar")}}, CreditBalance: 98532, AcceptThreshold: 3}),
	}},
	{Name: "KeyBook", Cases: []*TC{
		testdata.NewAcntTest(&KeyBook{Url: parseUrl("adi/book")}),
	}},
	{Name: "DataAccount", Cases: []*TC{
		testdata.NewAcntTest(&DataAccount{Url: parseUrl("adi/data"), AccountAuth: *simpleAuth}),
	}},
	{Name: "LiteDataAccount", Cases: []*TC{
		testdata.NewAcntTest(&LiteDataAccount{Url: parseUrl("lite-data-account"), Tail: []byte("asdf")}),
	}},
}

func parseUrl(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func txnTest(origin string, body TransactionBody) *TC {
	originUrl := parseUrl(origin)
	signer := new(signing.Builder)
	// In reality this would not work, but *shrug* it's a marshalling test
	signer.Type = SignatureTypeLegacyED25519
	signer.Url = originUrl
	signer.SetPrivateKey(key)
	signer.Version = 1
	signer.Timestamp = uint64(rand.Uint32())
	env := new(Envelope)
	txn := new(Transaction)
	env.Transaction = []*Transaction{txn}
	txn.Header.Principal = originUrl
	txn.Body = body

	sig, err := signer.Initiate(txn)
	if err != nil {
		panic(err)
	}

	env.Signatures = append(env.Signatures, sig)
	return testdata.NewTxnTest(env, body)
}

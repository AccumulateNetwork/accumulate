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

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
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
		txnTest("lite-token-account/ACME", &CreateIdentity{Url: parseUrl("adi"), PublicKey: key[32:]}),
		txnTest("lite-token-account/ACME", &CreateIdentity{Url: parseUrl("adi"), PublicKey: key[32:], KeyBookUrl: parseUrl("adi/book")}),
	}},
	{Name: "CreateTokenAccount", Cases: []*TC{
		txnTest("adi", &CreateTokenAccount{Url: parseUrl("adi/ACME"), TokenUrl: parseUrl("ACME")}),
		txnTest("adi", &CreateTokenAccount{Url: parseUrl("adi/ACME"), TokenUrl: parseUrl("ACME"), KeyBookUrl: parseUrl("adi/book")}),
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
		txnTest("adi", &WriteData{Entry: DataEntry{Data: []byte("foo"), ExtIds: [][]byte{[]byte("bar"), []byte("baz")}}}),
	}},
	{Name: "WriteDataTo", Cases: []*TC{
		txnTest("adi", &WriteDataTo{Recipient: parseUrl("lite-data-account"), Entry: DataEntry{Data: []byte("foo"), ExtIds: [][]byte{[]byte("bar"), []byte("baz")}}}),
	}},
	{Name: "AcmeFaucet", Cases: []*TC{
		txnTest("faucet", &AcmeFaucet{}),
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
		txnTest("adi", &CreateKeyPage{Keys: []*KeySpecParams{{PublicKey: key[32:]}}}),
	}},
	{Name: "CreateKeyBook", Cases: []*TC{
		txnTest("adi", &CreateKeyBook{Url: parseUrl("adi/book"), PublicKeyHash: key[32:]}),
	}},
	{Name: "AddCredits", Cases: []*TC{
		txnTest("lite-token-account", &AddCredits{Recipient: parseUrl("adi/page"), Amount: 100}),
	}},
	{Name: "UpdateKeyPage", Cases: []*TC{
		txnTest("adi", &UpdateKeyPage{Operation: &AddKeyOperation{Entry: KeySpecParams{PublicKey: key[32:]}}}),
	}},
	{Name: "SignPending", Cases: []*TC{
		txnTest("adi", &SignPending{}),
	}},
	{Name: "SyntheticCreateChain", Cases: []*TC{
		txnTest("adi", &SyntheticCreateChain{Cause: [32]byte{1}, Chains: []ChainParams{{Data: []byte{1, 2, 3}}}}),
	}},
	{Name: "SyntheticWriteData", Cases: []*TC{
		txnTest("adi", &SyntheticWriteData{Cause: [32]byte{1}, Entry: DataEntry{Data: []byte("foo"), ExtIds: [][]byte{[]byte("bar"), []byte("baz")}}}),
	}},
	{Name: "SyntheticDepositTokens", Cases: []*TC{
		txnTest("adi", &SyntheticDepositTokens{Cause: [32]byte{1}, Token: parseUrl("ACME"), Amount: *new(big.Int).SetInt64(10000)}),
	}},
	{Name: "SyntheticDepositCredits", Cases: []*TC{
		txnTest("adi", &SyntheticDepositCredits{Cause: [32]byte{1}, Amount: 1234}),
	}},
	{Name: "SyntheticBurnTokens", Cases: []*TC{
		txnTest("adi", &SyntheticBurnTokens{Cause: [32]byte{1}, Amount: *big.NewInt(123456789)}),
	}},
}

var acntTests = []*TCG{
	{Name: "Identity", Cases: []*TC{
		testdata.NewAcntTest(&ADI{AccountHeader: AccountHeader{Url: parseUrl("adi"), KeyBook: parseUrl("adi/book")}}),
		testdata.NewAcntTest(&ADI{AccountHeader: AccountHeader{Url: parseUrl("adi"), KeyBook: parseUrl("adi/book"), ManagerKeyBook: parseUrl("adi/mgr")}}),
	}},
	{Name: "TokenIssuer", Cases: []*TC{
		testdata.NewAcntTest(&TokenIssuer{AccountHeader: AccountHeader{Url: parseUrl("adi/foocoin"), KeyBook: parseUrl("adi/book")}, Symbol: "FOO", Precision: 10}),
	}},
	{Name: "TokenAccount", Cases: []*TC{
		testdata.NewAcntTest(&TokenAccount{AccountHeader: AccountHeader{Url: parseUrl("adi/foo"), KeyBook: parseUrl("adi/book")}, TokenUrl: parseUrl("adi/foocoin"), Balance: *big.NewInt(123456789)}),
	}},
	{Name: "LiteTokenAccount", Cases: []*TC{
		testdata.NewAcntTest(&LiteTokenAccount{AccountHeader: AccountHeader{Url: parseUrl("lite-token-account")}, TokenUrl: parseUrl("ACME"), Balance: *new(big.Int).SetInt64(12345), Nonce: 654, CreditBalance: *big.NewInt(9835)}),
	}},
	{Name: "KeyPage", Cases: []*TC{
		testdata.NewAcntTest(&KeyPage{AccountHeader: AccountHeader{Url: parseUrl("adi/page"), KeyBook: parseUrl("adi/book")}, Keys: []*KeySpec{{PublicKey: key[32:], Nonce: 651896, Owner: parseUrl("foo/bar")}}, CreditBalance: *big.NewInt(98532), Threshold: 3}),
	}},
	{Name: "KeyBook", Cases: []*TC{
		testdata.NewAcntTest(&KeyBook{AccountHeader: AccountHeader{Url: parseUrl("adi/book")}}),
	}},
	{Name: "DataAccount", Cases: []*TC{
		testdata.NewAcntTest(&DataAccount{AccountHeader: AccountHeader{Url: parseUrl("adi/data"), KeyBook: parseUrl("adi/book")}}),
	}},
	{Name: "LiteDataAccount", Cases: []*TC{
		testdata.NewAcntTest(&LiteDataAccount{AccountHeader: AccountHeader{Url: parseUrl("lite-data-account")}, Tail: []byte("asdf")}),
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
	signer := new(signing.Signer)
	// In reality this would not work, but *shrug* it's a marshalling test
	signer.Type = SignatureTypeLegacyED25519
	signer.Url = originUrl
	signer.PrivateKey = key
	signer.Height = 1
	signer.Timestamp = uint64(rand.Uint32())

	env := new(Envelope)
	txn := new(Transaction)
	env.Transaction = txn
	txn.Header.Principal = originUrl
	txn.Body = body

	sig, err := signer.Initiate(txn)
	if err != nil {
		panic(err)
	}

	env.Signatures = append(env.Signatures, sig)
	return testdata.NewTxnTest(env, body)
}

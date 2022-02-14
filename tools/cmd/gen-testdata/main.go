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
		txnTest1("lite-token-account/ACME", &CreateIdentity{Url: "adi", PublicKey: key[32:]}),
		txnTest1("lite-token-account/ACME", &CreateIdentity{Url: "adi", PublicKey: key[32:], KeyPageName: "page"}),
		txnTest1("lite-token-account/ACME", &CreateIdentity{Url: "adi", PublicKey: key[32:], KeyBookName: "book", KeyPageName: "page"}),
	}},
	{Name: "CreateTokenAccount", Cases: []*TC{
		txnTest1("adi", &CreateTokenAccount{Url: "adi/ACME", TokenUrl: "ACME"}),
		txnTest1("adi", &CreateTokenAccount{Url: "adi/ACME", TokenUrl: "ACME", KeyBookUrl: "adi/book"}),
		txnTest1("adi", &CreateTokenAccount{Url: "adi/ACME", TokenUrl: "ACME", Scratch: true}),
	}},
	{Name: "SendTokens", Cases: []*TC{
		txnTest1("adi/ACME", &SendTokens{To: []*TokenRecipient{{Url: "other/ACME", Amount: *new(big.Int).SetInt64(100)}}}),
		txnTest1("adi/ACME", &SendTokens{To: []*TokenRecipient{{Url: "other/ACME", Amount: *new(big.Int).SetInt64(100)}}, Meta: json.RawMessage(`{"foo":"bar"}`)}),
		txnTest1("adi/ACME", &SendTokens{To: []*TokenRecipient{{Url: "alice/ACME", Amount: *new(big.Int).SetInt64(100)}, {Url: "bob/ACME", Amount: *new(big.Int).SetInt64(100)}}}),
	}},
	{Name: "CreateDataAccount", Cases: []*TC{
		txnTest1("adi", &CreateDataAccount{Url: "adi/data"}),
	}},
	{Name: "WriteData", Cases: []*TC{
		txnTest1("adi", &WriteData{Entry: DataEntry{Data: []byte("foo"), ExtIds: [][]byte{[]byte("bar"), []byte("baz")}}}),
	}},
	{Name: "WriteDataTo", Cases: []*TC{
		txnTest1("adi", &WriteDataTo{Recipient: "lite-data-account", Entry: DataEntry{Data: []byte("foo"), ExtIds: [][]byte{[]byte("bar"), []byte("baz")}}}),
	}},
	{Name: "AcmeFaucet", Cases: []*TC{
		txnTest1("faucet", &AcmeFaucet{Url: "lite-token-account"}),
	}},
	{Name: "CreateToken", Cases: []*TC{
		txnTest1("adi", &CreateToken{Url: "adi/foocoin", Symbol: "FOO", Precision: 10}),
	}},
	{Name: "IssueTokens", Cases: []*TC{
		txnTest1("adi/foocoin", &IssueTokens{Recipient: "adi/foo", Amount: *new(big.Int).SetInt64(100)}),
	}},
	{Name: "BurnTokens", Cases: []*TC{
		txnTest1("adi/foo", &BurnTokens{Amount: *new(big.Int).SetInt64(100)}),
	}},
	{Name: "CreateKeyPage", Cases: []*TC{
		txnTest1("adi", &CreateKeyPage{Url: "adi/page", Keys: []*KeySpecParams{{PublicKey: key[32:]}}}),
	}},
	{Name: "CreateKeyBook", Cases: []*TC{
		txnTest1("adi", &CreateKeyBook{Url: "adi/book", Pages: []string{"adi/page"}}),
	}},
	{Name: "AddCredits", Cases: []*TC{
		txnTest1("lite-token-account", &AddCredits{Recipient: "adi/page", Amount: 100}),
	}},
	{Name: "UpdateKeyPage", Cases: []*TC{
		txnTest1("adi", &UpdateKeyPage{Operation: KeyPageOperationAdd, NewKey: key[32:]}),
	}},
	{Name: "SignPending", Cases: []*TC{
		txnTest1("adi", &SignPending{}),
	}},
	{Name: "SyntheticCreateChain", Cases: []*TC{
		txnTest1("adi", &SyntheticCreateChain{Cause: [32]byte{1}, Chains: []ChainParams{{Data: []byte{1, 2, 3}}}}),
	}},
	{Name: "SyntheticWriteData", Cases: []*TC{
		txnTest1("adi", &SyntheticWriteData{Cause: [32]byte{1}, Entry: DataEntry{Data: []byte("foo"), ExtIds: [][]byte{[]byte("bar"), []byte("baz")}}}),
	}},
	{Name: "SyntheticDepositTokens", Cases: []*TC{
		txnTest1("adi", &SyntheticDepositTokens{Cause: [32]byte{1}, Token: "ACME", Amount: *new(big.Int).SetInt64(10000)}),
	}},
	{Name: "SyntheticDepositCredits", Cases: []*TC{
		txnTest1("adi", &SyntheticDepositCredits{Cause: [32]byte{1}, Amount: 1234}),
	}},
	{Name: "SyntheticBurnTokens", Cases: []*TC{
		txnTest1("adi", &SyntheticBurnTokens{Cause: [32]byte{1}, Amount: *big.NewInt(123456789)}),
	}},
}

var acntTests = []*TCG{
	{Name: "Identity", Cases: []*TC{
		testdata.NewAcntTest(&ADI{AccountHeader: AccountHeader{Url: "adi", KeyBook: "adi/book"}}),
		testdata.NewAcntTest(&ADI{AccountHeader: AccountHeader{Url: "adi", KeyBook: "adi/book", ManagerKeyBook: "adi/mgr"}}),
	}},
	{Name: "TokenIssuer", Cases: []*TC{
		testdata.NewAcntTest(&TokenIssuer{AccountHeader: AccountHeader{Url: "adi/foocoin", KeyBook: "adi/book"}, Symbol: "FOO", Precision: 10}),
	}},
	{Name: "TokenAccount", Cases: []*TC{
		testdata.NewAcntTest(&TokenAccount{AccountHeader: AccountHeader{Url: "adi/foo", KeyBook: "adi/book"}, TokenUrl: "adi/foocoin", Balance: *big.NewInt(123456789)}),
	}},
	{Name: "LiteTokenAccount", Cases: []*TC{
		testdata.NewAcntTest(&LiteTokenAccount{AccountHeader: AccountHeader{Url: "lite-token-account"}, TokenUrl: "ACME", Balance: *new(big.Int).SetInt64(12345), Nonce: 654, CreditBalance: *big.NewInt(9835)}),
	}},
	{Name: "KeyPage", Cases: []*TC{
		testdata.NewAcntTest(&KeyPage{AccountHeader: AccountHeader{Url: "adi/page", KeyBook: "adi/book"}, Keys: []*KeySpec{{PublicKey: key[32:], Nonce: 651896, Owner: "foo/bar"}}, CreditBalance: *big.NewInt(98532), Threshold: 3}),
	}},
	{Name: "KeyBook", Cases: []*TC{
		testdata.NewAcntTest(&KeyBook{AccountHeader: AccountHeader{Url: "adi/book"}, Pages: []string{"adi/page"}}),
	}},
	{Name: "DataAccount", Cases: []*TC{
		testdata.NewAcntTest(&DataAccount{AccountHeader: AccountHeader{Url: "adi/data", KeyBook: "adi/book"}}),
	}},
	{Name: "LiteDataAccount", Cases: []*TC{
		testdata.NewAcntTest(&LiteDataAccount{AccountHeader: AccountHeader{Url: "lite-data-account"}, Tail: []byte("asdf")}),
	}},
}

func parseUrl(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func txnTest1(origin string, body TransactionPayload) *TC {
	return txnTest(&TransactionHeader{
		Origin:        parseUrl(origin),
		KeyPageHeight: 1,
		Nonce:         rand.Uint64(),
	}, body)
}

func txnTest(header *TransactionHeader, body TransactionPayload) *TC {
	env := new(Envelope)
	txn := new(Transaction)
	sig := new(ED25519Sig)

	env.Transaction = txn
	txn.TransactionHeader = *header
	txn.Body = body

	err := sig.Sign(header.Nonce, key, env.GetTxHash())
	if err != nil {
		panic(err)
	}

	env.Signatures = append(env.Signatures, sig)
	return testdata.NewTxnTest(env, body)
}

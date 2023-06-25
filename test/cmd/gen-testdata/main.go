// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

//lint:file-ignore ST1001 Don't care

import (
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	sdktest "gitlab.com/accumulatenetwork/accumulate/test/sdk"
	randPkg "golang.org/x/exp/rand"
)

// seed: m/44'/281'/0'/0'/0'
var keySeed, _ = hex.DecodeString("a2fd3e3b8c130edac176da83dcf809e22a01ab5a853560806e6cc054b3e160b0")
var key = ed25519.NewKeyFromSeed(keySeed[:])
var rand = randPkg.New(randPkg.NewSource(binary.BigEndian.Uint64(keySeed[:])))
var useSimpleHash = flag.Bool("simple", true, "Use simple hashes for signatures")

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

var cmdMain = &cobra.Command{
	Use:   "gen-testdata",
	Short: "Generate Accumulate test data",
	Run:   run,
}

var DataDir string

func run(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: gen-testdata <output-file> <ledger-test-vectors-file>")
		os.Exit(1)
	}

	file := args[0]

	// If the file is in a directory, make sure the directory exists
	dir := filepath.Dir(file)
	if dir != "" && dir != "." {
		check(os.MkdirAll(dir, 0755))
	}

	ts := &sdktest.TestSuite{
		Transactions: transactionTests(txnTest),
		Accounts:     accountTests(txnTest),
	}

	check(ts.Store(file))

	if len(args) > 1 {
		lts := &sdktest.TestSuite{
			Transactions: transactionTests(txnLedgerTestVectors),
			Accounts:     accountTests(txnLedgerTestVectors),
		}
		check(lts.Store(args[1]))
	}
}

func main() {
	cmdMain.Flags().StringVar(&DataDir, "corpus", "corpus", "Data output directory")
	_ = cmdMain.Execute()
}

type TCG = sdktest.TestCaseGroup
type TC = sdktest.TestCase

func transactionTests(txnTest func(*url.URL, TransactionBody) *TC) []*TCG {
	var txnTests = []*TCG{
		{Name: "CreateIdentity", Cases: []*TC{
			txnTest(AccountUrl("lite-token-account", "ACME"), &CreateIdentity{Url: AccountUrl("adi"), KeyHash: key[32:]}),
			txnTest(AccountUrl("lite-token-account", "ACME"), &CreateIdentity{Url: AccountUrl("adi"), KeyHash: key[32:], KeyBookUrl: AccountUrl("adi", "book")}),
		}},
		{Name: "CreateTokenAccount", Cases: []*TC{
			txnTest(AccountUrl("adi"), &CreateTokenAccount{Url: AccountUrl("adi", "ACME"), TokenUrl: AccountUrl("ACME")}),
			txnTest(AccountUrl("adi"), &CreateTokenAccount{Url: AccountUrl("adi", "ACME"), TokenUrl: AccountUrl("ACME"), Authorities: []*url.URL{AccountUrl("adi", "book")}}),
		}},
		{Name: "SendTokens", Cases: []*TC{
			txnTest(AccountUrl("adi", "ACME"), &SendTokens{To: []*TokenRecipient{{Url: AccountUrl("other", "ACME"), Amount: *new(big.Int).SetInt64(100)}}}),
			txnTest(AccountUrl("adi", "ACME"), &SendTokens{To: []*TokenRecipient{{Url: AccountUrl("other", "ACME"), Amount: *new(big.Int).SetInt64(100)}}, Meta: json.RawMessage(`{"foo":"bar"}`)}),
		}},
		{Name: "CreateDataAccount", Cases: []*TC{
			txnTest(AccountUrl("adi"), &CreateDataAccount{Url: AccountUrl("adi", "data")}),
		}},
		{Name: "WriteData", Cases: []*TC{
			txnTest(AccountUrl("adi"), &WriteData{Entry: &DoubleHashDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar"), []byte("baz")}}}),
		}},
		{Name: "WriteDataTo", Cases: []*TC{
			txnTest(AccountUrl("adi"), &WriteDataTo{Recipient: AccountUrl("lite-data-account"), Entry: &AccumulateDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar"), []byte("baz")}}}),
		}},
		{Name: "AcmeFaucet", Cases: []*TC{
			txnTest(AccountUrl("faucet"), &AcmeFaucet{Url: AccountUrl("lite-token-account")}),
		}},
		{Name: "CreateToken", Cases: []*TC{
			txnTest(AccountUrl("adi"), &CreateToken{Url: AccountUrl("adi", "foocoin"), Symbol: "FOO", Precision: 10}),
		}},
		{Name: "IssueTokens", Cases: []*TC{
			txnTest(AccountUrl("adi", "foocoin"), &IssueTokens{Recipient: AccountUrl("adi", "foo"), Amount: *new(big.Int).SetInt64(100)}),
		}},
		{Name: "BurnTokens", Cases: []*TC{
			txnTest(AccountUrl("adi", "foo"), &BurnTokens{Amount: *new(big.Int).SetInt64(100)}),
		}},
		{Name: "CreateKeyPage", Cases: []*TC{
			txnTest(AccountUrl("adi"), &CreateKeyPage{Keys: []*KeySpecParams{{KeyHash: key[32:]}}}),
		}},
		{Name: "CreateKeyBook", Cases: []*TC{
			txnTest(AccountUrl("adi"), &CreateKeyBook{Url: AccountUrl("adi", "book"), PublicKeyHash: key[32:]}),
		}},
		{Name: "AddCredits", Cases: []*TC{
			txnTest(AccountUrl("lite-token-account"), &AddCredits{Recipient: AccountUrl("adi", "page"), Amount: *big.NewInt(200000), Oracle: 50000000}),
		}},
		{Name: "UpdateKeyPage", Cases: []*TC{
			txnTest(AccountUrl("adi"), &UpdateKeyPage{Operation: []KeyPageOperation{&AddKeyOperation{Entry: KeySpecParams{KeyHash: key[32:]}}}}),
		}},
		{Name: "SignPending", Cases: []*TC{
			txnTest(AccountUrl("adi"), &RemoteTransaction{}),
		}},
		{Name: "SyntheticCreateIdentity", Cases: []*TC{
			txnTest(AccountUrl("adi"), &SyntheticCreateIdentity{SyntheticOrigin: SyntheticOrigin{Cause: PartitionUrl("X").WithTxID([32]byte{1})},
				Accounts: []Account{&UnknownAccount{Url: AccountUrl("foo")}}}),
		}},
		{Name: "SyntheticWriteData", Cases: []*TC{
			txnTest(AccountUrl("adi"), &SyntheticWriteData{SyntheticOrigin: SyntheticOrigin{Cause: PartitionUrl("X").WithTxID([32]byte{1})},
				Entry: &AccumulateDataEntry{Data: [][]byte{[]byte("foo"), []byte("bar"), []byte("baz")}}}),
		}},
		{Name: "SyntheticDepositTokens", Cases: []*TC{
			txnTest(AccountUrl("adi"), &SyntheticDepositTokens{SyntheticOrigin: SyntheticOrigin{Cause: PartitionUrl("X").WithTxID([32]byte{1})},
				Token: AccountUrl("ACME"), Amount: *new(big.Int).SetInt64(10000)}),
		}},
		{Name: "SyntheticDepositCredits", Cases: []*TC{
			txnTest(AccountUrl("adi"), &SyntheticDepositCredits{SyntheticOrigin: SyntheticOrigin{Cause: PartitionUrl("X").WithTxID([32]byte{1})}, Amount: 1234}),
		}},
		{Name: "SyntheticBurnTokens", Cases: []*TC{
			txnTest(AccountUrl("adi"), &SyntheticBurnTokens{SyntheticOrigin: SyntheticOrigin{Cause: PartitionUrl("X").WithTxID([32]byte{1})},
				Amount: *big.NewInt(123456789)}),
		}},
	}

	return txnTests
}

func accountTests(txnTest func(*url.URL, TransactionBody) *TC) []*TCG {
	var simpleAuth = &AccountAuth{Authorities: []AuthorityEntry{{Url: AccountUrl("adi", "book")}}}
	var managerAuth = &AccountAuth{Authorities: []AuthorityEntry{{Url: AccountUrl("adi", "book"), Disabled: true}, {Url: AccountUrl("adi", "mgr")}}}

	var acntTests = []*TCG{
		{Name: "Identity", Cases: []*TC{
			sdktest.NewAcntTest(&ADI{Url: AccountUrl("adi"), AccountAuth: *simpleAuth}),
			sdktest.NewAcntTest(&ADI{Url: AccountUrl("adi"), AccountAuth: *managerAuth}),
		}},
		{Name: "TokenIssuer", Cases: []*TC{
			sdktest.NewAcntTest(&TokenIssuer{Url: AccountUrl("adi", "foocoin"), AccountAuth: *simpleAuth, Symbol: "FOO", Precision: 10}),
		}},
		{Name: "TokenAccount", Cases: []*TC{
			sdktest.NewAcntTest(&TokenAccount{Url: AccountUrl("adi", "foo"), AccountAuth: *simpleAuth, TokenUrl: AccountUrl("adi", "foocoin"), Balance: *big.NewInt(123456789)}),
		}},
		{Name: "LiteTokenAccount", Cases: []*TC{
			sdktest.NewAcntTest(&LiteTokenAccount{Url: AccountUrl("lite-token-account"), TokenUrl: AccountUrl("ACME"), Balance: *big.NewInt(12345)}),
		}},
		{Name: "LiteIdentity", Cases: []*TC{
			sdktest.NewAcntTest(&LiteIdentity{Url: AccountUrl("lite-identity"), LastUsedOn: uint64(rand.Uint32()), CreditBalance: 9835}),
		}},
		{Name: "KeyPage", Cases: []*TC{
			sdktest.NewAcntTest(&KeyPage{Url: AccountUrl("adi", "page"), Keys: []*KeySpec{{PublicKeyHash: key[32:], LastUsedOn: uint64(rand.Uint32()), Delegate: AccountUrl("foo", "bar")}}, CreditBalance: 98532, AcceptThreshold: 3}),
		}},
		{Name: "KeyBook", Cases: []*TC{
			sdktest.NewAcntTest(&KeyBook{Url: AccountUrl("adi", "book")}),
		}},
		{Name: "DataAccount", Cases: []*TC{
			sdktest.NewAcntTest(&DataAccount{Url: AccountUrl("adi", "data"), AccountAuth: *simpleAuth}),
		}},
		{Name: "LiteDataAccount", Cases: []*TC{
			sdktest.NewAcntTest(&LiteDataAccount{Url: AccountUrl("lite-data-account")}),
		}},
	}

	return acntTests
}

func txnTest(originUrl *url.URL, body TransactionBody) *TC {
	signer := new(signing.Builder)
	// In reality this would not work, but *shrug* it's a marshalling test
	signer.Type = SignatureTypeED25519
	signer.Url = originUrl
	signer.SetPrivateKey(key)
	signer.Version = 1
	//provide a deterministic timestamp
	signer.SetTimestamp(uint64(1234567890))
	env := new(messaging.Envelope)
	txn := new(Transaction)
	env.Transaction = []*Transaction{txn}
	txn.Header.Principal = originUrl
	txn.Body = body

	if *useSimpleHash {
		signer.InitMode = signing.InitWithSimpleHash
	}

	sig, err := signer.Initiate(txn)
	if err != nil {
		panic(err)
	}

	env.Signatures = append(env.Signatures, sig)
	return sdktest.NewTxnTest(env)
}

func txnLedgerTestVectors(originUrl *url.URL, body TransactionBody) *TC {
	signer := new(signing.Builder)
	lts := LedgerSignerTester{}
	lts.PubKey = key.Public().(ed25519.PublicKey)

	signer.Signer = &lts

	// this is not a real transaction, only used for test vector generation
	signer.Type = SignatureTypeED25519
	signer.Url = originUrl
	signer.Version = 1
	signer.SetTimestamp(uint64(1234567890))

	env := new(messaging.Envelope)
	txn := new(Transaction)
	env.Transaction = []*Transaction{txn}
	txn.Header.Principal = originUrl
	txn.Body = body

	if *useSimpleHash {
		signer.InitMode = signing.InitWithSimpleHash
	}

	sig, err := signer.Initiate(txn)
	if err != nil && !errors.Is(err, UseHardwareError) {
		panic(err)
	}

	env.Signatures = append(env.Signatures, sig)
	if txn.Body.Type() == TransactionTypeSendTokens {
		data, _ := env.MarshalBinary()
		fmt.Println(hex.EncodeToString(data))
	}
	return sdktest.NewTxnTest(env)
}

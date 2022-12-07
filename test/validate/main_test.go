package validate

import (
	"crypto/ed25519"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TODO: Provide a flag to run against a real network

func TestValidate(t *testing.T) {
	// Set up the simulator
	net := simulator.SimpleNetwork(t.Name(), 3, 1)
	sim, err := simulator.New(
		acctesting.NewTestLogger(t),
		simulator.MemoryDatabase,
		net,
		simulator.Genesis(GenesisTime),
	)
	require.NoError(t, err)

	// Set up the test suite
	s := new(ValidationTestSuite)
	s.Harness = New(t, sim.Services(), sim)

	// Faucet with IssueTokens
	s.faucet = func(recipient, amount any) build.SignatureBuilder {
		b := build.Transaction().For("ACME").
			IssueTokens(amount, AcmePrecisionPower).To(recipient).
			SignWith(DnUrl(), "operators", "1").Version(1).Timestamp(1).PrivateKey(net.Bvns[0].Nodes[0].PrivValKey)
		for i, bvn := range net.Bvns {
			for j, node := range bvn.Nodes {
				if i == 0 && j == 0 {
					continue
				}
				b = b.SignWith(DnUrl(), "operators", "1").Version(1).Timestamp(1).PrivateKey(node.PrivValKey)
			}
		}
		return b
	}

	// Run
	suite.Run(t, s)
}

type ValidationTestSuite struct {
	suite.Suite
	*Harness
	nonce  uint64
	faucet func(recipient, amount any) build.SignatureBuilder
}

func (s *ValidationTestSuite) SetupTest() {
	s.Harness.TB = s.T()
}

func (s *ValidationTestSuite) TestMain() {
	// Set up lite addresses
	liteKey := acctesting.GenerateKey("Lite")
	liteAcme := acctesting.AcmeLiteAddressStdPriv(liteKey)
	liteId := liteAcme.RootIdentity()

	// Set up the ADI and its keys
	adi := AccountUrl("test")
	key10 := acctesting.GenerateKey(1, 0)
	key20 := acctesting.GenerateKey(2, 0)
	key21 := acctesting.GenerateKey(2, 1)
	key22 := acctesting.GenerateKey(2, 2)
	key23 := acctesting.GenerateKey(2, 3) // Orig
	key24 := acctesting.GenerateKey(2, 4) // New
	key30 := acctesting.GenerateKey(3, 0)
	key31 := acctesting.GenerateKey(3, 1)
	keymgr := acctesting.GenerateKey("mgr")

	// Generate a Lite Token Account
	st := s.BuildAndSubmitSuccessfully(s.faucet(liteAcme, 1e6))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*LiteTokenAccount](s.Harness, liteAcme).Balance)

	// Add credits to lite account
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			AddCredits().To(liteId).WithOracle(InitialAcmeOracle).Purchase(1e6).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*LiteIdentity](s.Harness, liteId).CreditBalance)

	// Create an ADI
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			CreateIdentity(adi).WithKey(key10, SignatureTypeED25519).WithKeyBook(adi, "book").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	QueryAccountAs[*ADI](s.Harness, adi)

	// Recreating an ADI fails and the synthetic transaction is recorded
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			CreateIdentity(adi).WithKey(key10, SignatureTypeED25519).WithKeyBook(adi, "book").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Fails())

	// Add credits to the ADI's key page 1
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			AddCredits().To(adi, "book", "1").WithOracle(InitialAcmeOracle).Purchase(6e4).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "1")).CreditBalance)

	// Create additional Key Pages
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book").
			CreateKeyPage().WithEntry().Key(key20, SignatureTypeED25519).FinishEntry().
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2"))

	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book").
			CreateKeyPage().WithEntry().Key(key30, SignatureTypeED25519).FinishEntry().
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "3"))

	// Add credits to the ADI's key page 2
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			AddCredits().To(adi, "book", "2").WithOracle(InitialAcmeOracle).Purchase(1e3).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).CreditBalance)

	// Attempting to lock key page 2 using itself fails
	st = s.BuildAndSubmit(
		build.Transaction().For(adi, "book", "2").
			UpdateKeyPage().UpdateAllowed().Deny(TransactionTypeUpdateKeyPage).FinishOperation().
			SignWith(adi, "book", "2").Version(1).Timestamp(&s.nonce).PrivateKey(key20))

	_ = s.NotNil(st.Error) &&
		s.Equal("signature 0: acc://test.acme/book/2 cannot modify its own allowed operations", st.Error.Message)

	s.Nil(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).TransactionBlacklist)

	// Lock key page 2 using page 1
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book", "2").
			UpdateKeyPage().UpdateAllowed().Deny(TransactionTypeUpdateKeyPage).FinishOperation().
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.NotNil(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).TransactionBlacklist)

	// Attempting to update key page 3 using page 2 fails
	st = s.BuildAndSubmit(
		build.Transaction().For(adi, "book", "3").
			UpdateKeyPage().Add().Entry().Key(key31, SignatureTypeED25519).FinishEntry().FinishOperation().
			SignWith(adi, "book", "2").Version(1).Timestamp(&s.nonce).PrivateKey(key20))

	_ = s.NotNil(st.Error) &&
		s.Equal("signature 0: page acc://test.acme/book/2 is not authorized to sign updateKeyPage", st.Error.Message)

	s.Len(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "3")).Keys, 1)

	// Unlock key page 2 using page 1
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book", "2").
			UpdateKeyPage().UpdateAllowed().Allow(TransactionTypeUpdateKeyPage).FinishOperation().
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Nil(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).TransactionBlacklist)

	// Update key page 3 using page 2
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book", "3").
			UpdateKeyPage().Add().Entry().Key(key31, SignatureTypeED25519).FinishEntry().FinishOperation().
			SignWith(adi, "book", "2").Version(3).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Len(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "3")).Keys, 2)

	// Add keys to page 2
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book", "2").
			UpdateKeyPage().
			Add().Entry().Key(key21, SignatureTypeED25519).FinishEntry().FinishOperation().
			Add().Entry().Key(key22, SignatureTypeED25519).FinishEntry().FinishOperation().
			Add().Entry().Key(key23, SignatureTypeED25519).FinishEntry().FinishOperation().
			SignWith(adi, "book", "2").Version(3).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Len(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).Keys, 4)

	// Update key page entry with same keyhash different delegate
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi).
			CreateKeyBook(adi, "book2").WithKey(key20, SignatureTypeED25519).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book2", "1"))

	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			AddCredits().To(adi, "book2", "1").WithOracle(InitialAcmeOracle).Purchase(1e3).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book2", "1")).CreditBalance)

	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book2", "1").
			UpdateKeyPage().Update().
			Entry().Key(key20, SignatureTypeED25519).FinishEntry().
			To().Key(key20, SignatureTypeED25519).Owner(adi, "book").FinishEntry().
			FinishOperation().
			SignWith(adi, "book2", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.NotNil(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book2", "1")).Keys[0].Delegate)

	// Set KeyBook2 as authority for adi token account
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi).
			CreateTokenAccount(adi, "acmetokens").ForToken(ACME).WithAuthority(adi, "book2").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book2", "1").Version(2).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Equal(adi.JoinPath("book2").String(), QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("acmetokens")).Authorities[0].Url.String())

	// Burn Tokens (?) for adi token account
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			SendTokens(0.01, AcmePrecisionPower).To(adi, "acmetokens").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.NotZero(QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("acmetokens")).Balance)

	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "acmetokens").
			BurnTokens(0.01, AcmePrecisionPower).
			SignWith(adi, "book2", "1").Version(2).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Zero(QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("acmetokens")).Balance)

	// Set KeyBook2 as authority for adi data account
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi).
			CreateTokenAccount(adi, "testdata1").ForToken(ACME).WithAuthority(adi, "book2").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book2", "1").Version(2).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Equal(adi.JoinPath("book2").String(), QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("testdata1")).Authorities[0].Url.String())

	_, _ = key24, keymgr

	// Set threshold to 2 of 2
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book", "2").
			UpdateKeyPage().SetThreshold(2).
			SignWith(adi, "book", "2").Version(4).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Equal(2, int(QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2")).AcceptThreshold))

	// Set threshold to 0 of 0
	st = s.BuildAndSubmit(
		build.Transaction().For(adi, "book", "2").
			UpdateKeyPage().SetThreshold(0).
			SignWith(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key20))

	_ = s.NotNil(st.Error) &&
		s.Equal("cannot require 0 signatures on a key page", st.Error.Message)

	// Update a key with only that key's signature
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "book", "2").
			UpdateKey(key24, SignatureTypeED25519).
			SignWith(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key23))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	page := QueryAccountAs[*KeyPage](s.Harness, adi.JoinPath("book", "2"))
	doesNotHaveKey(s.T(), page, key23, SignatureTypeED25519)
	hasKey(s.T(), page, key24, SignatureTypeED25519)

	// Create an ADI Token Account
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi).
			CreateTokenAccount(adi, "tokens").ForToken(ACME).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("tokens"))

	// Send tokens from the lite token account to the ADI token account
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			SendTokens(5, AcmePrecisionPower).To(adi, "tokens").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.Require().Equal("5.00000000", FormatBigAmount(&QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("tokens")).Balance, AcmePrecisionPower))

	// Send tokens from the ADI token account to the lite token account using the multisig page
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "tokens").
			SendTokens(1, AcmePrecisionPower).To(liteAcme).
			SignWith(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	// Signing the transaction with the same key does not deliver it
	st = s.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key20))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	s.Require().NotZero(s.QueryPending(adi.JoinPath("tokens"), nil).Total)

	// Sign the pending transaction using the other key
	st = s.BuildAndSubmitSuccessfully(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key21))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// Signing the transaction after it has been delivered fails
	st = s.BuildAndSubmit(
		build.SignatureForTxID(st.TxID).
			Url(adi, "book", "2").Version(5).Timestamp(&s.nonce).PrivateKey(key22))

	h := st.TxID.Hash()
	_ = s.NotNil(st.Error) &&
		s.Equal(fmt.Sprintf("transaction %x (sendTokens) has been delivered", h[:4]), st.Error.Message)

	// Create a token issuer
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi).
			CreateToken(adi, "token-issuer").WithSymbol("TOK").WithPrecision(10).WithSupplyLimit(1000000).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	QueryAccountAs[*TokenIssuer](s.Harness, adi.JoinPath("token-issuer"))

	// Issue tokens
	liteTok := liteId.JoinPath(adi.Authority, "token-issuer")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "token-issuer").
			IssueTokens("123.0123456789", 10).To(liteTok).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.Require().Equal("123.0123456789", FormatBigAmount(&QueryAccountAs[*LiteTokenAccount](s.Harness, liteTok).Balance, 10))

	// Burn tokens
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteTok).
			BurnTokens(100, 10).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.Require().Equal("23.0123456789", FormatBigAmount(&QueryAccountAs[*LiteTokenAccount](s.Harness, liteTok).Balance, 10))

	// Create lite data account and write the data
	fde := &FactomDataEntry{ExtIds: [][]byte{[]byte("Factom PRO"), []byte("Tutorial")}}
	fde.AccountId = *(*[32]byte)(ComputeLiteDataAccountId(fde.Wrap()))
	lda, err := LiteDataAddress(fde.AccountId[:])
	s.Require().NoError(err)
	s.Require().Equal("acc://b36c1c4073305a41edc6353a094329c24ffa54c0a47fb56227a04477bcb78923", lda.String(), "Account ID is wrong")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(lda).
			WriteData().Entry(fde.Wrap()).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	st = s.QueryTransaction(st.TxID, nil).Status
	s.Require().IsType((*WriteDataResult)(nil), st.Result)
	wdr := st.Result.(*WriteDataResult)
	s.Require().Equal(lda.String(), wdr.AccountUrl.String())
	s.Require().NotZero(wdr.EntryHash)
	s.Require().NotEmpty(wdr.AccountID)

	// Create ADI Data Account
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi).
			CreateDataAccount(adi, "data").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	// Write data to ADI Data Account
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "data").
			WriteData([]byte("foo"), []byte("bar")).Scratch().
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	st = s.QueryTransaction(st.TxID, nil).Status
	s.Require().IsType((*WriteDataResult)(nil), st.Result)
	wdr = st.Result.(*WriteDataResult)
	s.Require().NotZero(wdr.EntryHash)

	// Create a sub ADI
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi).
			CreateIdentity(adi, "sub1").WithKeyBook(adi, "sub1", "book").WithKey(key10, SignatureTypeED25519).
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	// Create another ADI (manager)
	manager := AccountUrl("manager")
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			CreateIdentity(manager).WithKey(keymgr, SignatureTypeED25519).WithKeyBook(manager, "book").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// Add credits to manager's key page 1
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			AddCredits().To(manager, "book", "1").WithOracle(InitialAcmeOracle).Purchase(1e3).
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// Create token account with manager
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi).
			CreateTokenAccount(adi, "managed-tokens").ForToken(ACME).WithAuthority(adi, "book").WithAuthority(manager, "book").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitSuccessfully(
		build.SignatureForTransaction(s.QueryTransaction(st.TxID, nil).Transaction).
			Url(manager, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(keymgr))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Require().Len(QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("managed-tokens")).Authorities, 2)

	// Remove manager from token account
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "managed-tokens").
			UpdateAccountAuth().Remove(manager, "book").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitSuccessfully(
		build.SignatureForTransaction(s.QueryTransaction(st.TxID, nil).Transaction).
			Url(manager, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(keymgr))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Require().Len(QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("managed-tokens")).Authorities, 1)

	// Add manager to token account
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(adi, "managed-tokens").
			UpdateAccountAuth().Add(manager, "book").
			SignWith(adi, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(key10))
	s.StepUntil(
		Txn(st.TxID).IsPending())

	st = s.BuildAndSubmitSuccessfully(
		build.SignatureForTransaction(s.QueryTransaction(st.TxID, nil).Transaction).
			Url(manager, "book", "1").Version(1).Timestamp(&s.nonce).PrivateKey(keymgr))
	s.StepUntil(
		Txn(st.TxID).Succeeds())

	s.Require().Len(QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("managed-tokens")).Authorities, 2)

	// Transaction with Memo
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			Memo("hello world").
			SendTokens(5, AcmePrecisionPower).To(adi, "tokens").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	s.Require().Equal("hello world", s.QueryTransaction(st.TxID, nil).Transaction.Header.Memo)

	// Refund on expensive synthetic txn failure
	creditsBefore := QueryAccountAs[*LiteIdentity](s.Harness, liteId).CreditBalance
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			CreateIdentity(adi).WithKey(key10, SignatureTypeED25519).WithKeyBook(adi, "book").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Fails(),
		Txn(st.TxID).Refund().Succeeds())
	creditsAfter := QueryAccountAs[*LiteIdentity](s.Harness, liteId).CreditBalance
	s.Require().Equal(100, int(creditsBefore-creditsAfter))

	tokensBefore := QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("tokens")).Balance.Int64()
	st = s.BuildAndSubmitSuccessfully(
		build.Transaction().For(liteAcme).
			SendTokens(5, AcmePrecisionPower).To("invalid-account").
			SignWith(liteId).Version(1).Timestamp(&s.nonce).PrivateKey(liteKey))
	s.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Fails(),
		Txn(st.TxID).Refund().Succeeds())
	tokensAfter := QueryAccountAs[*TokenAccount](s.Harness, adi.JoinPath("tokens")).Balance.Int64()
	s.Require().Equal(int(tokensBefore), int(tokensAfter))
}

func hasKey(tb testing.TB, page *KeyPage, key ed25519.PrivateKey, typ SignatureType) {
	tb.Helper()
	hash, err := PublicKeyHash(key[32:], typ)
	require.NoError(tb, err)
	_, _, ok := page.EntryByKeyHash(hash)
	require.Truef(tb, ok, "%v should have %x", page.Url, key[32:])
}

func doesNotHaveKey(tb testing.TB, page Signer2, key ed25519.PrivateKey, typ SignatureType) {
	tb.Helper()
	hash, err := PublicKeyHash(key[32:], typ)
	require.NoError(tb, err)
	_, _, ok := page.EntryByKeyHash(hash)
	require.Falsef(tb, ok, "%v should not have %x", page.GetUrl(), key[32:])
}

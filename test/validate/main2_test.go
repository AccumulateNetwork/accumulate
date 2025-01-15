// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package validate

import (
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/suite"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestValidate runs the validation test suite against the simulator.
func TestValidate2(t *testing.T) {
	suite.Run(t, new(Validation2TestSuite))
}

type Validation2TestSuite struct {
	suite.Suite
	*Harness
	sim *simulator.Simulator

	// faucetSvc is a service that will give tokens to an account
	faucetSvc api.Faucet

	// nonce is a nonce/timestamp used for signatures
	nonce uint64

	// Add state here
}

type urlAndKey struct {
	Url *url.URL
	Key ed25519.PrivateKey
}

func (s *Validation2TestSuite) newAdiKey(urlStr string) urlAndKey {
	// Parse the URL
	u := url.MustParse(urlStr)

	// Create a key in a way that is repeatable
	k := acctesting.GenerateKey(u)

	return urlAndKey{u, k}
}

func (s *Validation2TestSuite) newLiteAccount(seed ...any) urlAndKey {
	// Create a key in a way that is repeatable
	k := acctesting.GenerateKey(seed...)

	// Format the lite address
	u := LiteAuthorityForKey(k[32:], SignatureTypeED25519)

	return urlAndKey{u, k}
}

// faucet is a helper function for calling the faucet service.
func (s *Validation2TestSuite) faucet(account *url.URL) *protocol.TransactionStatus {
	sub, err := s.faucetSvc.Faucet(context.Background(), account, api.FaucetOptions{})
	s.Require().NoError(err)
	return sub.Status
}

// getOracle returns the credit oracle, necessary for buying credits.
func (s *Validation2TestSuite) getOracle() float64 {
	ns := s.NetworkStatus(api.NetworkStatusOptions{Partition: protocol.Directory})
	return float64(ns.Oracle.Price) / AcmeOraclePrecision
}

// SetupSuite is run once.
func (s *Validation2TestSuite) SetupSuite() {
	s.sim, s.faucetSvc = setupSim(s.T(), simulator.NewSimpleNetwork(s.T().Name(), 3, 1))
	s.Harness = New(s.T(), s.sim.Services(), s.sim)
}

// SetupTest is run for each test.
func (s *Validation2TestSuite) SetupTest() {
	s.Harness.TB = s.T()
}

func (s *Validation2TestSuite) TestCreateADI() {
	// Using the test name when generating lite accounts will ensure they are
	// unique and avoids contamination between tests.
	//
	// The only verification step that is strictly necessary is the one that
	// verifies the functionality being tested, e.g. creating an ADI. Verifying
	// balances is not necessary, but it is helpful because it gives you
	// immediate feedback if something goes wrong.
	//
	// Waiting for transactions to complete _is_ necessary any time the next
	// step requires the outputs of the previous step.

	// Key for a lite account
	lite := s.newLiteAccount(s.T().Name())
	liteAcme := lite.Url.JoinPath(ACME)

	// Key for an ADI
	adi := s.newAdiKey("TestCreateADI.acme")

	// Ask the faucet for some tokens
	st := s.faucet(liteAcme)
	s.StepUntil(Txn(st.TxID).Completes())                                     // Wait until the transaction completes
	s.NotZero(QueryAccountAs[*LiteTokenAccount](s.Harness, liteAcme).Balance) // Verify the balance is now non-zero

	// Buy some credits
	st = s.BuildAndSubmitTxnSuccessfully(build.Transaction().
		For(liteAcme).             // Build a transaction for the lite token account
		AddCredits().              // To buy credits
		To(lite.Url).              // For the lite identity
		WithOracle(s.getOracle()). // Get the current oracle
		Purchase(1e6).             // Purchase 1 million credits
		SignWith(lite.Url).        // Sign with the lite identity
		Version(1).                // Lite accounts always have a version of 1
		Timestamp(&s.nonce).       // Set the timestamp/nonce
		PrivateKey(lite.Key))      // Sign with the lite identity key
	s.StepUntil(Txn(st.TxID).Completes())
	s.NotZero(QueryAccountAs[*LiteIdentity](s.Harness, lite.Url).CreditBalance)

	// Create an ADI
	st = s.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(liteAcme).
			CreateIdentity(adi.Url).
			WithKeyBook(adi.Url, "book").
			WithKey(adi.Key.Public(), SignatureTypeED25519).
			SignWith(lite.Url).Version(1).Timestamp(&s.nonce).PrivateKey(lite.Key))
	s.StepUntil(Txn(st.TxID).Completes())

	// Verify success (verify the ADI exists)
	QueryAccountAs[*ADI](s.Harness, adi.Url)
}

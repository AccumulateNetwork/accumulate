package e2e

import (
	"crypto/ed25519"
	"sync"

	"github.com/stretchr/testify/suite"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	testing "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
	"gitlab.com/accumulatenetwork/accumulate/types/state"
	"golang.org/x/exp/rand"
)

type NewDUT func(*Suite) DUT

// DUT are the parameters needed to test the Device Under Test.
type DUT interface {
	GetRecordAs(url string, target state.Chain)
	GetRecordHeight(url string) uint64
	SubmitTxn(*transactions.Envelope)
	WaitForTxns(...[]byte)
}

type Suite struct {
	suite.Suite
	start NewDUT
	dut   DUT
	rand  *rand.Rand

	synthMu *sync.Mutex
	synthTx map[[32]byte]*url.URL
}

var _ suite.SetupTestSuite = (*Suite)(nil)

func NewSuite(start NewDUT) *Suite {
	s := new(Suite)
	s.start = start
	return s
}

func (s *Suite) SetupTest() {
	s.dut = s.start(s)
	s.rand = rand.New(rand.NewSource(0))
	s.synthMu = new(sync.Mutex)
	s.synthTx = map[[32]byte]*url.URL{}
}

func (s *Suite) generateKey() ed25519.PrivateKey {
	_, key, _ := ed25519.GenerateKey(s.rand)
	return key
}

func (s *Suite) generateTmKey() tmed25519.PrivKey {
	return tmed25519.PrivKey(s.generateKey())
}

func (s *Suite) newTx(sponsor *url.URL, key tmed25519.PrivKey, nonce uint64, body protocol.TransactionPayload) *transactions.Envelope {
	s.T().Helper()
	return testing.NewTransaction().
		WithOrigin(sponsor).
		WithKeyPage(0, s.dut.GetRecordHeight(sponsor.String())).
		WithNonce(nonce).
		WithBody(body).
		SignLegacyED25519(key)
}

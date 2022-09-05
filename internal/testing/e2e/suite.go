package e2e

import (
	"crypto/ed25519"
	"sync"

	"github.com/stretchr/testify/suite"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/rand"
)

type NewDUT func(*Suite) DUT

// DUT are the parameters needed to test the Device Under Test.
type DUT interface {
	GetRecordAs(url string, target protocol.Account)
	GetRecordHeight(url string) uint64
	SubmitTxn(*protocol.Envelope)
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

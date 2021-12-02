package e2e

import (
	"crypto/ed25519"
	"encoding"
	"sync"

	"github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/stretchr/testify/suite"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	"golang.org/x/exp/rand"
)

type NewDUT func(*Suite) DUT

// DUT are the parameters needed to test the Device Under Test.
type DUT interface {
	GetUrl(url string) (*ctypes.ResultABCIQuery, error)
	SubmitTxn(*transactions.GenTransaction)
	WaitForTxns()
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

func (s *Suite) newTx(sponsor *url.URL, key tmed25519.PrivKey, nonce uint64, body encoding.BinaryMarshaler) *transactions.GenTransaction {
	s.T().Helper()
	tx, err := transactions.New(sponsor.String(), func(hash []byte) (*transactions.ED25519Sig, error) {
		sig := new(transactions.ED25519Sig)
		return sig, sig.Sign(nonce, key, hash)
	}, body)
	s.Require().NoError(err)
	return tx
}

func (s *Suite) getChainAs(url string, obj encoding.BinaryUnmarshaler) {
	s.T().Helper()
	r, err := s.dut.GetUrl(url)

	s.Require().NoError(err)
	s.Require().Zero(r.Response.Code, "Query failed: %v", r.Response.Info)
	so := state.Object{}
	s.Require().NoError(so.UnmarshalBinary(r.Response.Value))
	s.Require().NoError(obj.UnmarshalBinary(so.Entry))
}

func (s *Suite) parseUrl(str string) *url.URL {
	u, err := url.Parse(str)
	s.Require().NoError(err)
	return u
}

func (s *Suite) liteUrl(key tmed25519.PrivKey) *url.URL {
	u, err := protocol.LiteAddress(key.PubKey().Bytes(), protocol.ACME)
	s.Require().NoError(err)
	return u
}

func (s *Suite) deposit(sponsor, recipient tmed25519.PrivKey) {
	tx, err := testing.CreateFakeSyntheticDepositTx(sponsor, recipient)
	s.Require().NoError(err)
	s.dut.SubmitTxn(tx)
	s.dut.WaitForTxns()
}

func (s *Suite) createADI(sponsor *url.URL, sponsorKey tmed25519.PrivKey, nonce uint64, adi string, adiKey tmed25519.PrivKey) {
	ic := new(protocol.IdentityCreate)
	ic.Url = adi
	ic.PublicKey = adiKey.PubKey().Bytes()
	ic.KeyBookName = "key0"
	ic.KeyPageName = "key0-0"

	tx := s.newTx(sponsor, sponsorKey, nonce, ic)
	s.dut.SubmitTxn(tx)
	s.dut.WaitForTxns()
}

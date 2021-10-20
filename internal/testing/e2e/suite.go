package e2e

import (
	"crypto/ed25519"
	"encoding"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AccumulateNetwork/accumulated/internal/api"
	"github.com/AccumulateNetwork/accumulated/internal/relay"
	acctesting "github.com/AccumulateNetwork/accumulated/internal/testing"
	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	apitypes "github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/stretchr/testify/suite"
	abci "github.com/tendermint/tendermint/abci/types"
	tmed25519 "github.com/tendermint/tendermint/crypto/ed25519"
	"golang.org/x/exp/rand"
)

type Suite struct {
	suite.Suite
	start func(*Suite) *api.Query
	query *api.Query
	rand  *rand.Rand

	synthMu *sync.Mutex
	synthTx map[[32]byte]*url.URL
}

func NewSuite(start func(*Suite) *api.Query) *Suite {
	s := new(Suite)
	s.start = start
	return s
}

func (s *Suite) SetupTest() {
	s.query = s.start(s)
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
	tx, err := transactions.New(sponsor.String(), func(hash []byte) (*transactions.ED25519Sig, error) {
		sig := new(transactions.ED25519Sig)
		return sig, sig.Sign(nonce, key, hash)
	}, body)
	s.Require().NoError(err)
	return tx
}

func (s *Suite) getChainAs(url string, obj encoding.BinaryUnmarshaler) {
	r, err := s.query.QueryByUrl(url)
	s.Require().NoError(err)
	s.Require().Zero(r.Response.Code, "Query failed: %v", r.Response.Info)
	s.Require().NoError(obj.UnmarshalBinary(r.Response.Value))
}

func (s *Suite) sendTxAsync(tx *transactions.GenTransaction) func(relay.BatchedStatus) {
	done := make(chan abci.TxResult)
	ti, err := s.query.BroadcastTx(tx, done)
	s.Require().NoError(err)

	return func(bs relay.BatchedStatus) {
		r, err := bs.ResolveTransactionResponse(ti)
		s.Require().NoError(err)
		s.Require().Zero(r.Code, "TX failed: %s", r.Log)
		s.Require().Empty(r.MempoolError, "TX failed: %s", r.MempoolError)

		var txr abci.TxResult
		select {
		case txr = <-done:
			s.Require().Zerof(txr.Result.Code, "TX failed: %s", txr.Result.Log)
		case <-time.After(1 * time.Minute):
			s.T().Fatal("Timed out while waiting for TX repsonse")
		}

		for _, e := range txr.Result.Events {
			if e.Type != "accSyn" {
				continue
			}

			var id [32]byte
			var u *url.URL
			for _, a := range e.Attributes {
				switch a.Key {
				case "txRef":
					b, err := hex.DecodeString(a.Value)
					if s.NoError(err) {
						copy(id[:], b)
					}
				case "url":
					u, err = url.Parse(a.Value)
					s.NoError(err)
				}
			}

			if id != ([32]byte{}) && u != nil {
				s.synthMu.Lock()
				s.synthTx[id] = u
				s.synthMu.Unlock()
			}
		}
	}
}

func (s *Suite) waitForSynth() {
	for {
		s.synthMu.Lock()
		if len(s.synthTx) == 0 {
			s.synthMu.Unlock()
			return
		}

		var id [32]byte
		var u *url.URL
		for id, u = range s.synthTx {
		}
		delete(s.synthTx, id)
		s.synthMu.Unlock()

		// Poll for TX results. This is hacky, but it's a test.
		for {
			r, err := s.query.GetTx(u.Routing(), id)
			if err == nil {
				s.Require().Zero(r.TxResult.Code, "TX failed: %s", r.TxResult.Log)
				break
			}

			if !strings.Contains(err.Error(), "not found") {
				s.Require().NoError(err)
				break
			}

			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (s *Suite) TestCreateAnonAccount() {
	sponsor, sender := s.generateTmKey(), s.generateTmKey()
	tx, err := acctesting.CreateFakeSyntheticDepositTx(sponsor, sender)
	s.Require().NoError(err)
	s.sendTxAsync(tx)(<-s.query.BatchSend())

	senderUrl, err := protocol.AnonymousAddress(sender.PubKey().Bytes(), protocol.ACME)
	s.Require().NoError(err)

	recipients := make([]*url.URL, 10)
	for i := range recipients {
		key := s.generateTmKey()
		u, err := protocol.AnonymousAddress(key.PubKey().Bytes(), protocol.ACME)
		s.Require().NoError(err)
		recipients[i] = u
	}

	var fns []func(relay.BatchedStatus)
	var total int64
	for i := 0; i < 10; i++ {
		if i > 2 && testing.Short() {
			break
		}

		exch := apitypes.NewTokenTx(types.String(senderUrl.String()))
		for i := 0; i < 10; i++ {
			if i > 2 && testing.Short() {
				break
			}
			recipient := recipients[s.rand.Intn(len(recipients))]
			exch.AddToAccount(types.String(recipient.String()), 1000)
			total += 1000
		}

		tx := s.newTx(senderUrl, sender, uint64(i+1), exch)
		fn := s.sendTxAsync(tx)
		fns = append(fns, fn)
	}

	bs := <-s.query.BatchSend()
	for _, fn := range fns {
		fn(bs)
	}

	s.waitForSynth()

	account := new(protocol.AnonTokenAccount)
	s.getChainAs(senderUrl.String(), account)
	s.Require().Equal(int64(5e4*acctesting.TokenMx-total), account.Balance.Int64())
}

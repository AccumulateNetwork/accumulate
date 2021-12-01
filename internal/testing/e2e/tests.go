package e2e

import (
	"testing"

	"github.com/AccumulateNetwork/accumulate/internal/relay"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	apitypes "github.com/AccumulateNetwork/accumulate/types/api"
)

func (s *Suite) TestGenesis() {
	s.Run("ACME", func() {
		acme := new(protocol.TokenIssuer)
		s.getChainAs(protocol.AcmeUrl().String(), acme)
		s.Assert().Equal(protocol.AcmeUrl().String(), string(acme.ChainUrl))
		s.Assert().Equal(protocol.ACME, string(acme.Symbol))
	})
}

func (s *Suite) TestCreateAnonAccount() {
	sponsor, sender := s.generateTmKey(), s.generateTmKey()

	senderUrl, err := protocol.AnonymousAddress(sender.PubKey().Bytes(), protocol.ACME)
	s.Require().NoError(err)

	tx, err := acctesting.CreateFakeSyntheticDepositTx(sponsor, sender)
	s.Require().NoError(err)
	s.sendTxAsync(tx)(<-s.query.BatchSend())

	s.waitForSynth()

	account := new(protocol.LiteTokenAccount)
	s.getChainAs(senderUrl.String(), account)
	s.Require().Equal(int64(5e4*acctesting.TokenMx), account.Balance.Int64())

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

	account = new(protocol.LiteTokenAccount)
	s.getChainAs(senderUrl.String(), account)
	s.Require().Equal(int64(5e4*acctesting.TokenMx-total), account.Balance.Int64())
}

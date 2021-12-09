package e2e

import (
	"testing"

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

func (s *Suite) TestCreateLiteAccount() {
	sponsor, sender := s.generateTmKey(), s.generateTmKey()

	senderUrl, err := protocol.LiteAddress(sender.PubKey().Bytes(), protocol.ACME)
	s.Require().NoError(err)

	tx, err := acctesting.CreateFakeSyntheticDepositTx(sponsor, sender)
	s.Require().NoError(err)
	s.dut.SubmitTxn(tx)
	s.dut.WaitForTxns(tx.TransactionHash())

	account := new(protocol.LiteTokenAccount)
	s.getChainAs(senderUrl.String(), account)
	s.Require().Equal(int64(5e4*acctesting.TokenMx), account.Balance.Int64())

	recipients := make([]*url.URL, 10)
	for i := range recipients {
		key := s.generateTmKey()
		u, err := protocol.LiteAddress(key.PubKey().Bytes(), protocol.ACME)
		s.Require().NoError(err)
		recipients[i] = u
	}

	var total int64
	var txids [][]byte
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
		s.dut.SubmitTxn(tx)
		txids = append(txids, tx.TransactionHash())
	}

	s.dut.WaitForTxns(txids...)

	account = new(protocol.LiteTokenAccount)
	s.getChainAs(senderUrl.String(), account)
	s.Require().Equal(int64(5e4*acctesting.TokenMx-total), account.Balance.Int64())
}

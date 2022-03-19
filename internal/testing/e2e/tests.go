package e2e

import (
	"math/big"
	"testing"

	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (s *Suite) TestGenesis() {
	s.Run("ACME", func() {
		acme := new(protocol.TokenIssuer)
		s.dut.GetRecordAs(protocol.AcmeUrl().String(), acme)
		s.Assert().Equal(protocol.AcmeUrl(), acme.Url)
		s.Assert().Equal(protocol.ACME, string(acme.Symbol))
	})
}

func (s *Suite) TestCreateLiteAccount() {
	sender := s.generateTmKey()

	senderUrl, err := protocol.LiteTokenAddress(sender.PubKey().Bytes(), protocol.ACME)
	s.Require().NoError(err)

	tx, err := acctesting.CreateFakeSyntheticDepositTx(sender)
	s.Require().NoError(err)
	s.dut.SubmitTxn(tx)
	s.dut.WaitForTxns(tx.GetTxHash())

	account := new(protocol.LiteTokenAccount)
	s.dut.GetRecordAs(senderUrl.String(), account)
	s.Require().Equal(int64(acctesting.TestTokenAmount*acctesting.TokenMx), account.Balance.Int64())

	var nonce uint64 = 1
	amtAcmeToBuyCredits := int64(10 * protocol.AcmePrecision)
	tx = acctesting.NewTransaction().
		WithPrincipal(senderUrl).
		WithSigner(senderUrl, s.dut.GetRecordHeight(senderUrl.String())).
		WithNonce(nonce).
		WithBody(&protocol.AddCredits{Recipient: senderUrl, Amount: *big.NewInt(amtAcmeToBuyCredits)}).
		Initiate(protocol.SignatureTypeLegacyED25519, sender)
	s.dut.SubmitTxn(tx)
	s.dut.WaitForTxns(tx.GetTxHash())

	recipients := make([]*url.URL, 10)
	for i := range recipients {
		key := s.generateTmKey()
		u, err := protocol.LiteTokenAddress(key.PubKey().Bytes(), protocol.ACME)
		s.Require().NoError(err)
		recipients[i] = u
	}

	var total int64
	var txids [][]byte
	for i := 0; i < 10; i++ {
		if i > 2 && testing.Short() {
			break
		}

		exch := new(protocol.SendTokens)
		for i := 0; i < 10; i++ {
			if i > 2 && testing.Short() {
				break
			}
			recipient := recipients[s.rand.Intn(len(recipients))]
			exch.AddRecipient(recipient, big.NewInt(int64(1000)))
			total += 1000
		}

		nonce++
		tx := acctesting.NewTransaction().
			WithPrincipal(senderUrl).
			WithSigner(senderUrl, s.dut.GetRecordHeight(senderUrl.String())).
			WithNonce(nonce).
			WithBody(exch).
			Initiate(protocol.SignatureTypeLegacyED25519, sender)
		s.dut.SubmitTxn(tx)
		txids = append(txids, tx.GetTxHash())
	}
	total += amtAcmeToBuyCredits

	s.dut.WaitForTxns(txids...)

	account = new(protocol.LiteTokenAccount)
	s.dut.GetRecordAs(senderUrl.String(), account)
	s.Require().Equal(int64(3e5*acctesting.TokenMx-total), account.Balance.Int64())
}

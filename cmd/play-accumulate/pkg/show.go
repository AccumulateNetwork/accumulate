package pkg

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (s *Session) Dump(v interface{}) {
	spew.Fdump(s.Stdout, v)
}

func (s *Session) Print(v interface{}) {
	if s, ok := v.(string); ok && !strings.HasSuffix(s, "\n") {
		v = s + "\n"
	}
	fmt.Fprint(s.Stdout, v)
}

func (s *Session) tryGetTokenAccount(url Urlish) protocol.AccountWithTokens {
	account, err := s.Engine.GetAccount(s.url(url))
	if err != nil {
		return nil
	}

	token, ok := account.(protocol.AccountWithTokens)
	if !ok {
		return nil
	}

	return token
}

func (s *Session) tryGetTokenIssuer(url Urlish) *protocol.TokenIssuer {
	account, err := s.Engine.GetAccount(s.url(url))
	if err != nil {
		return nil
	}

	token, ok := account.(*protocol.TokenIssuer)
	if !ok {
		return nil
	}

	return token
}

func (s *Session) formatBalanceForIssuer(url Urlish, balance *big.Int) string {
	issuer := s.tryGetTokenIssuer(s.url(url))
	if issuer == nil {
		return balance.String()
	}
	return protocol.FormatBigAmount(balance, int(issuer.Precision)) + " " + issuer.Symbol
}

func (s *Session) formatBalanceForAccount(url Urlish, balance *big.Int) string {
	account := s.tryGetTokenAccount(s.url(url))
	if account == nil {
		return balance.String()
	}
	return s.formatBalanceForIssuer(account.GetTokenUrl(), balance)
}

func (s *Session) Show(v interface{}) {
	if ctxn, ok := v.(*completedTxn); ok {
		v = ctxn.Transaction
	}

	txn, ok := v.(*protocol.Transaction)
	if ok {
		v = txn.Body
	}

	switch v := v.(type) {
	case string:
		s.Print(v)
	case *URL:
		s.Print(v.String())

	// Accounts
	case *protocol.LiteTokenAccount:
		s.Print(fmt.Sprintf(
			"Lite Token Account\n"+
				"    Identity:   %v\n"+
				"    Issuer:     %v\n"+
				"    Balance:    %v\n"+
				"    Credits:    %v\n",
			v.Url.Authority,
			v.TokenUrl.ShortString(),
			s.formatBalanceForIssuer(v.TokenUrl, &v.Balance),
			protocol.FormatAmount(v.CreditBalance, protocol.CreditPrecisionPower),
		))
	case protocol.Account:
		s.Print(fmt.Sprintf(
			"Account %v (%v)\n",
			v.GetUrl(), v.Type(),
		))

	// Transactions
	case *protocol.SendTokens:
		str := fmt.Sprintf(
			"Send Tokens Transaction\n"+
				"    Hash:   %X\n"+
				"    From:   %v\n",
			txn.GetHash(),
			txn.Header.Principal,
		)
		for _, to := range v.To {
			str += fmt.Sprintf("    Send:   %v to %v\n", s.formatBalanceForAccount(to.Url, &to.Amount), to.Url)
		}
		s.Print(str)
	case *protocol.AddCredits:
		s.Print(fmt.Sprintf(
			"Add Credits Transaction\n"+
				"    Hash:    %X\n"+
				"    From:    %v\n"+
				"    To:      %v\n"+
				"    Spent:   %v\n",
			txn.GetHash(),
			txn.Header.Principal,
			v.Recipient,
			protocol.FormatBigAmount(&v.Amount, protocol.AcmePrecisionPower),
		))
	case *protocol.AcmeFaucet:
		s.Print(fmt.Sprintf(
			"ACME Faucet Transaction\n"+
				"    Hash:     %X\n"+
				"    To:       %v\n"+
				"    Amount:   %d ACME\n",
			txn.GetHash(),
			v.Url,
			protocol.AcmeFaucetAmount,
		))
	case protocol.TransactionBody:
		s.Print(fmt.Sprintf(
			"Transaction %X (%v) for %v\n",
			txn.GetHash(), v.Type(), txn.Header.Principal,
		))

	default:
		s.Print(spew.Sdump(v))
	}
}

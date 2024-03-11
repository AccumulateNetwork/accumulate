// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pkg

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (s *Session) Dump(v interface{}) {
	s.Output(Output{"text/plain", []byte(spew.Sdump(v))})
}

func (s *Session) Print(v interface{}) {
	if s, ok := v.(string); ok && !strings.HasSuffix(s, "\n") {
		v = s + "\n"
	}
	s.Output(Output{"text/plain", []byte(fmt.Sprint(v))})
}

func (s *Session) printAndJSON(str string, v interface{}) {
	if !strings.HasSuffix(str, "\n") {
		str = str + "\n"
	}
	out := []Output{{"text/plain", []byte(str)}}
	if json, err := json.Marshal(v); err == nil {
		out = append(out, Output{"application/json", json})
	}
	s.Output(out...)
}

func (s *Session) formatBalanceForIssuer(url Urlish, balance *big.Int) string {
	var issuer *protocol.TokenIssuer
	if !s.TryGetAccountAs(url, &issuer) {
		return balance.String()
	}
	return protocol.FormatBigAmount(balance, int(issuer.Precision)) + " " + issuer.Symbol
}

func (s *Session) formatBalanceForAccount(url Urlish, balance *big.Int) string {
	var account protocol.AccountWithTokens
	if !s.TryGetAccountAs(url, &account) {
		return balance.String()
	}
	return s.formatBalanceForIssuer(account.GetTokenUrl(), balance)
}

func (s *Session) Show(v interface{}) {
	txn, ok := v.(*protocol.Transaction)
	if ok {
		v = txn.Body
	}

	var str string
	switch v := v.(type) {
	case string:
		str = v
	case *URL:
		str = v.String()

	case completedFlow:
		for _, txn := range v {
			s.Show(txn)
		}
		return

	case *completedTxn:
		s.Show(v.Transaction)
		return

	// Accounts
	case *protocol.LiteIdentity:
		dir, _ := s.TryGetDirectory(v.Url)
		str = fmt.Sprintf(
			"Lite Identity\n"+
				"    Url:       %v\n"+
				"    Credits:    %v\n",
			v.Url,
			protocol.FormatAmount(v.CreditBalance, protocol.CreditPrecisionPower))
		for i, url := range dir {
			str += fmt.Sprintf("    Entry %d:   %s\n", i+1, url)
		}
	case *protocol.LiteTokenAccount:
		str = fmt.Sprintf(
			"Lite Token Account\n"+
				"    Identity:   %v\n"+
				"    Issuer:     %v\n"+
				"    Balance:    %v\n",
			v.Url.Authority,
			v.TokenUrl.ShortString(),
			s.formatBalanceForIssuer(v.TokenUrl, &v.Balance),
		)
	case *protocol.ADI:
		dir, _ := s.TryGetDirectory(v.Url)
		str = fmt.Sprintf(
			"Identity (ADI)\n"+
				"    Url:       %v\n",
			v.Url,
		)
		for i, url := range dir {
			str += fmt.Sprintf("    Entry %d:   %s\n", i+1, url)
		}
	case *protocol.TokenAccount:
		str = fmt.Sprintf(
			"Token Account\n"+
				"    Url:        %v\n"+
				"    Issuer:     %v\n"+
				"    Balance:    %v\n",
			v.Url,
			v.TokenUrl.ShortString(),
			s.formatBalanceForIssuer(v.TokenUrl, &v.Balance),
		)
	case *protocol.KeyBook:
		str = fmt.Sprintf(
			"Key Book\n"+
				"    Url:      %v\n",
			v.Url,
		)
		for i := uint64(0); i < v.PageCount; i++ {
			str += fmt.Sprintf("    Page %d:   %s\n", i+1, protocol.FormatKeyPageUrl(v.Url, i))
		}
	case *protocol.KeyPage:
		str = fmt.Sprintf(
			"Key Page\n"+
				"    Url:          %v\n"+
				"    Credits:      %v\n"+
				"    Version:      %v\n"+
				"    Thresholds:   accept=%d, reject=%d, response=%d, block=%d\n",
			v.Url.Authority,
			protocol.FormatAmount(v.CreditBalance, protocol.CreditPrecisionPower),
			v.Version,
			v.AcceptThreshold, v.RejectThreshold, v.ResponseThreshold, v.BlockThreshold,
		)
		for i, key := range v.Keys {
			switch {
			case key.Delegate == nil:
				str += fmt.Sprintf("    Key %d:        %X\n", i+1, key.PublicKeyHash)
			case key.PublicKeyHash == nil:
				str += fmt.Sprintf("    Key %d:        %v\n", i+1, key.Delegate)
			default:
				str += fmt.Sprintf("    Key %d:        %X (%v)\n", i+1, key.PublicKeyHash, key.Delegate)
			}
		}
	case *protocol.TokenIssuer:
		str = fmt.Sprintf(
			"Token Issuer\n"+
				"    Url:         %v\n"+
				"    Symbol:      %v\n"+
				"    Precision:   %v\n"+
				"    Issued:      %v\n",
			v.Url,
			v.Symbol,
			v.Precision,
			protocol.FormatBigAmount(&v.Issued, int(v.Precision)),
		)
		if v.SupplyLimit != nil {
			str += fmt.Sprintf(
				"    Limit:       %v\n",
				protocol.FormatBigAmount(v.SupplyLimit, int(v.Precision)),
			)
		}
	case protocol.Account:
		str = fmt.Sprintf(
			"Account %v (%v)\n",
			v.GetUrl(), v.Type(),
		)

	// Transactions
	case *protocol.SendTokens:
		str = fmt.Sprintf(
			"Send Tokens Transaction\n"+
				"    Hash:   %X\n"+
				"    From:   %v\n",
			txn.GetHash(),
			txn.Header.Principal,
		)
		for _, to := range v.To {
			str += fmt.Sprintf("    Send:   %v to %v\n", s.formatBalanceForAccount(to.Url, &to.Amount), to.Url)
		}
	case *protocol.AddCredits:
		str = fmt.Sprintf(
			"Add Credits Transaction\n"+
				"    Hash:    %X\n"+
				"    From:    %v\n"+
				"    To:      %v\n"+
				"    Spent:   %v\n",
			txn.GetHash(),
			txn.Header.Principal,
			v.Recipient,
			protocol.FormatBigAmount(&v.Amount, protocol.AcmePrecisionPower),
		)
	case *protocol.AcmeFaucet:
		str = fmt.Sprintf(
			"ACME Faucet Transaction\n"+
				"    Hash:     %X\n"+
				"    To:       %v\n"+
				"    Amount:   %d ACME\n",
			txn.GetHash(),
			v.Url,
			protocol.AcmeFaucetAmount,
		)
	case protocol.TransactionBody:
		str = fmt.Sprintf(
			"Transaction %X (%v) for %v\n",
			txn.GetHash(), v.Type(), txn.Header.Principal,
		)

	default:
		str = spew.Sdump(v)
	}
	s.printAndJSON(str, v)
}

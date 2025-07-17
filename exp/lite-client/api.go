package liteclient

import (
	"context"
	"fmt"

	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	v2 "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TokenAccountAPI defines methods for querying token account data in the lite client.
type TokenAccountAPI interface {
	// GetBalance returns the balance for a token account.
	GetBalance(ctx context.Context, accountUrl string) (BalanceResult, error)
	// GetTransactions returns up to 'limit' transactions for a token account (all if limit<=0).
	GetTransactions(ctx context.Context, accountUrl string, limit int) ([]TransactionResult, error)
	// GetBalanceAndTransactions returns both the balance and up to 'limit' transactions.
	GetBalanceAndTransactions(ctx context.Context, accountUrl string, limit int) (BalanceResult, []TransactionResult, error)
}

// BalanceResult holds token balance and metadata.
type BalanceResult struct {
	AccountUrl string
	Balance    string // Use string or big.Int depending on protocol
	Token      string
	Height     int64
}

// TransactionResult holds transaction details.
type TransactionResult struct {
	TxID      string
	Type      string
	Timestamp int64
	Amount    string // Use string or big.Int as needed
	From      string
	To        string
	Status    string
}

// Implementation of TokenAccountAPI on LiteClient

// GetBalance returns the balance for a token account.
func (c *LiteClient) GetBalance(ctx context.Context, accountUrl string) (BalanceResult, error) {
	// Try v3 first
	u, err := accurl.Parse(accountUrl)
	if err != nil {
		return BalanceResult{}, fmt.Errorf("invalid account URL: %w", err)
	}

	q := api.Querier2{Querier: c.v3}
	resp, err := q.QueryAccount(ctx, u, nil)
	if err != nil {
		fmt.Printf("[DEBUG] v3 QueryAccount error: %v\n", err)
	}
	if err == nil && resp != nil {
		// Only handle token accounts
		tok, ok := resp.Account.(*protocol.TokenAccount)
		if ok {
			chain, err := q.QueryChain(ctx, u, &api.ChainQuery{Name: "main"})
			var height int64
			if err == nil && chain != nil {
				height = int64(chain.Count)
			}
			return BalanceResult{
				AccountUrl: accountUrl,
				Balance:    tok.Balance.String(),
				Token:      tok.TokenUrl.String(),
				Height:     height,
			}, nil
		}
	}

	// Fallback to v2
	if c.v2 != nil {
		v2req := &v2.UrlQuery{Url: u}
		v2resp := new(v2.ChainQueryResponse)
		err2 := c.v2.RequestAPIv2(ctx, "query", v2req, v2resp)
		if err2 != nil {
			fmt.Printf("[DEBUG] v2 fallback decode error: %v\n", err2)
		}
		if err2 == nil && v2resp.Data != nil {
			// The v2 response for a token account is a general account response
			ta, ok := v2resp.Data.(*protocol.TokenAccount)
			if ok {
				return BalanceResult{
					AccountUrl: accountUrl,
					Balance:    ta.Balance.String(),
					Token:      ta.TokenUrl.String(),
					Height:     int64(v2resp.MainChain.Height),
				}, nil
			}
		}
	}

	return BalanceResult{}, fmt.Errorf("failed to query account balance: %w", err)
}

// GetTransactions returns up to 'limit' transactions for a token account (all if limit<=0).
func (c *LiteClient) GetTransactions(ctx context.Context, accountUrl string, limit int) ([]TransactionResult, error) {
	u, err := accurl.Parse(accountUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	q := api.Querier2{Querier: c.v3}
	rangeOpts := &api.RangeOptions{FromEnd: true}
	if limit > 0 {
		count := uint64(limit)
		rangeOpts.Count = &count
	}

	chainRec, err := q.QueryMainChainEntries(ctx, u, &api.ChainQuery{Name: "main", Range: rangeOpts})
	if err != nil {
		fmt.Printf("[DEBUG] v3 QueryMainChainEntries error: %v\n", err)
		return nil, err
	}

	results := make([]TransactionResult, 0, len(chainRec.Records))
	for _, entry := range chainRec.Records {
		msgRec := entry.Value
		if msgRec.Error != nil {
			continue // Or log error
		}

		txid := msgRec.Message.ID()

		txMsg := msgRec.Message

		var amount, from, to string
		if wd, ok := txMsg.Transaction.Body.(*protocol.SendTokens); ok {
			if len(wd.To) > 0 {
				amount = wd.To[0].Amount.String()
				to = wd.To[0].Url.String()
			}
			from = txMsg.Transaction.Header.Principal.String()
		}

		status := msgRec.Status.String()

		var timestamp int64
		if msgRec.LastBlockTime != nil {
			timestamp = msgRec.LastBlockTime.Unix()
		}

		results = append(results, TransactionResult{
			TxID:      txid.String(),
			Type:      txMsg.Transaction.Body.Type().String(),
			Timestamp: timestamp,
			Amount:    amount,
			From:      from,
			To:        to,
			Status:    status,
		})
	}
	return results, nil
}

// GetBalanceAndTransactions returns both the balance and up to 'limit' transactions.
func (c *LiteClient) GetBalanceAndTransactions(ctx context.Context, accountUrl string, limit int) (BalanceResult, []TransactionResult, error) {
	bal, err := c.GetBalance(ctx, accountUrl)
	if err != nil {
		return bal, nil, err
	}
	txs, err := c.GetTransactions(ctx, accountUrl, limit)
	return bal, txs, err
}

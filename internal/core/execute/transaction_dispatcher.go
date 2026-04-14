// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TransactionPortion describes the work a shard must perform for a transaction.
// It lists the accounts owned by this shard that the transaction touches.
type TransactionPortion struct {
	// IsPrimary is true when this shard owns the transaction's principal account.
	IsPrimary bool

	// Accounts lists the account URLs routed to this shard.
	Accounts []*url.URL
}

// SyntheticDispatch describes a synthetic transaction that must be sent to a
// remote shard (or partition) after the originating transaction executes.
type SyntheticDispatch struct {
	// Type is the synthetic transaction type that will be produced.
	Type protocol.TransactionType

	// Destination is the account that will receive the synthetic transaction.
	Destination *url.URL
}

// DispatchResult is returned by RouteTransaction and contains the shard
// mapping plus any synthetic dispatches the transaction will produce.
type DispatchResult struct {
	// Portions maps shard ID to the set of accounts that shard must handle.
	Portions map[int]*TransactionPortion

	// Synthetics lists synthetic transactions this transaction will produce.
	Synthetics []SyntheticDispatch
}

// TransactionDispatcher routes transactions to shards based on the accounts
// they touch. It does not execute transactions; it only determines dispatch
// paths.
type TransactionDispatcher struct {
	shardDepth int // bits used for shard routing (e.g. 6 for 64 shards)
}

// NewTransactionDispatcher creates a dispatcher with the given shard depth.
// The number of shards is 2^shardDepth.
func NewTransactionDispatcher(shardDepth int) *TransactionDispatcher {
	return &TransactionDispatcher{shardDepth: shardDepth}
}

// NumShards returns the total number of shards.
func (d *TransactionDispatcher) NumShards() int {
	return 1 << d.shardDepth
}

// RouteToShard maps an account URL to its shard index using ADI hash routing,
// matching the BPT sharding: identityAccountID[0] >> (8 - shardDepth).
// All accounts under the same ADI route to the same shard.
func (d *TransactionDispatcher) RouteToShard(account *url.URL) int {
	id := account.IdentityAccountID32()
	return int(id[0] >> (8 - d.shardDepth))
}

// RouteTransaction analyzes a transaction to determine which shards are
// involved and what synthetic transactions will be produced. It does not
// execute anything.
func (d *TransactionDispatcher) RouteTransaction(txn *protocol.Transaction) *DispatchResult {
	result := &DispatchResult{
		Portions: make(map[int]*TransactionPortion),
	}

	// The principal account is always touched.
	principal := txn.Header.Principal
	if principal != nil {
		d.addAccount(result, principal, true)
	}

	// Extract additional accounts and synthetics from the transaction body.
	d.analyzeBody(result, principal, txn.Body)

	return result
}

// addAccount adds an account to the dispatch result, routing it to its shard.
func (d *TransactionDispatcher) addAccount(result *DispatchResult, account *url.URL, isPrimary bool) {
	if account == nil {
		return
	}
	shard := d.RouteToShard(account)
	portion, ok := result.Portions[shard]
	if !ok {
		portion = &TransactionPortion{}
		result.Portions[shard] = portion
	}
	if isPrimary {
		portion.IsPrimary = true
	}
	// Avoid duplicates.
	for _, existing := range portion.Accounts {
		if existing.Equal(account) {
			return
		}
	}
	portion.Accounts = append(portion.Accounts, account)
}

// analyzeBody examines the transaction body to identify additional accounts
// touched and synthetic transactions produced. This is the core routing logic.
func (d *TransactionDispatcher) analyzeBody(result *DispatchResult, principal *url.URL, body protocol.TransactionBody) {
	switch b := body.(type) {

	// --- Token operations ---

	case *protocol.SendTokens:
		// Recipients receive synthetic deposit tokens.
		for _, to := range b.To {
			if to.Url != nil {
				result.Synthetics = append(result.Synthetics, SyntheticDispatch{
					Type:        protocol.TransactionTypeSyntheticDepositTokens,
					Destination: to.Url,
				})
			}
		}

	case *protocol.BurnTokens:
		// Burns principal only; produces synthetic burn to issuer if needed.
		// The issuer URL is not in the body (resolved at execution time),
		// so we cannot route it here.

	case *protocol.IssueTokens:
		// Recipient and To list receive synthetic deposits.
		if b.Recipient != nil {
			result.Synthetics = append(result.Synthetics, SyntheticDispatch{
				Type:        protocol.TransactionTypeSyntheticDepositTokens,
				Destination: b.Recipient,
			})
		}
		for _, to := range b.To {
			if to.Url != nil {
				result.Synthetics = append(result.Synthetics, SyntheticDispatch{
					Type:        protocol.TransactionTypeSyntheticDepositTokens,
					Destination: to.Url,
				})
			}
		}

	case *protocol.AddCredits:
		// Credits are deposited to the recipient account.
		if b.Recipient != nil {
			result.Synthetics = append(result.Synthetics, SyntheticDispatch{
				Type:        protocol.TransactionTypeSyntheticDepositCredits,
				Destination: b.Recipient,
			})
		}

	case *protocol.TransferCredits:
		for _, to := range b.To {
			if to.Url != nil {
				d.addAccount(result, to.Url, false)
			}
		}

	// --- Account creation ---

	case *protocol.CreateIdentity:
		// The new identity URL is created.
		if b.Url != nil {
			d.addAccount(result, b.Url, false)
		}
		if b.KeyBookUrl != nil {
			d.addAccount(result, b.KeyBookUrl, false)
		}

	case *protocol.CreateTokenAccount:
		if b.Url != nil {
			d.addAccount(result, b.Url, false)
		}

	case *protocol.CreateDataAccount:
		if b.Url != nil {
			d.addAccount(result, b.Url, false)
		}

	case *protocol.CreateToken:
		if b.Url != nil {
			d.addAccount(result, b.Url, false)
		}

	case *protocol.CreateKeyBook:
		if b.Url != nil {
			d.addAccount(result, b.Url, false)
		}

	case *protocol.CreateKeyPage:
		// Key pages are sub-accounts of the key book (principal).
		// The exact URL is constructed at execution time, so we only
		// touch the principal here.

	// --- Data operations ---

	case *protocol.WriteData:
		// Writes to principal only, no additional accounts.

	case *protocol.WriteDataTo:
		// Produces a synthetic write data to the recipient.
		if b.Recipient != nil {
			result.Synthetics = append(result.Synthetics, SyntheticDispatch{
				Type:        protocol.TransactionTypeSyntheticWriteData,
				Destination: b.Recipient,
			})
		}

	// --- Auth operations ---

	case *protocol.UpdateAccountAuth:
		// Operations may reference authority URLs; the actual URLs are
		// inside the operations, which vary by type. The principal is
		// always touched. Authority lookups happen at execution time.

	case *protocol.UpdateKeyPage:
		// Modifies the principal key page only.

	case *protocol.UpdateKey:
		// Modifies the principal key page only.

	case *protocol.SetLiteAccountDelegate:
		// Touches the delegate key book if set, for validation.
		if b.Delegate != nil {
			d.addAccount(result, b.Delegate, false)
		}

	// --- Account operations ---

	case *protocol.LockAccount:
		// Touches principal only.

	case *protocol.BurnCredits:
		// Touches principal only.

	case *protocol.AcmeFaucet:
		// Produces a synthetic deposit to the target lite account.
		if b.Url != nil {
			result.Synthetics = append(result.Synthetics, SyntheticDispatch{
				Type:        protocol.TransactionTypeSyntheticDepositTokens,
				Destination: b.Url,
			})
		}

	// --- Synthetic transactions (dispatched to destination) ---

	case *protocol.SyntheticDepositTokens:
		// Touches the principal (destination) only.

	case *protocol.SyntheticDepositCredits:
		// Touches the principal (destination) only.

	case *protocol.SyntheticCreateIdentity:
		// Touches the principal (destination) only.

	case *protocol.SyntheticWriteData:
		// Touches the principal (destination) only.

	case *protocol.SyntheticBurnTokens:
		// Touches the principal (destination) only.

	// --- System transactions ---

	case *protocol.DirectoryAnchor, *protocol.BlockValidatorAnchor, *protocol.SystemGenesis:
		// System transactions are handled specially; not shard-routed.

	default:
		// Unknown transaction types: only the principal is touched (already added).
	}
}

// DispatchBlock groups a batch of transactions by shard, returning a slice
// indexed by shard ID. Each entry contains the transactions routed to that
// shard (by their principal account). This is the primary entry point for
// block-level dispatch.
func (d *TransactionDispatcher) DispatchBlock(txns []*protocol.Transaction) [][]int {
	shards := make([][]int, d.NumShards())
	for i, txn := range txns {
		if txn.Header.Principal == nil {
			continue
		}
		shard := d.RouteToShard(txn.Header.Principal)
		shards[shard] = append(shards[shard], i)
	}
	return shards
}

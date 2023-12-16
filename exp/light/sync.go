// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package light

import (
	"bytes"
	"context"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slog"
)

// PullAccount fetches the latest version of the account and its chains.
func (c *Client) PullAccount(ctx context.Context, acctUrl *url.URL) error {
	return c.PullAccountWithChains(ctx, acctUrl, func(*api.ChainRecord) bool { return true })
}

func (c *Client) PullAccountWithChains(ctx context.Context, acctUrl *url.URL, predicate func(*api.ChainRecord) bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	batch := c.OpenDB(true)
	defer batch.Discard()

	// Update the account
	r1, err := c.query.QueryAccount(ctx, acctUrl, nil)
	if err != nil {
		return errors.UnknownError.WithFormat("query account: %w", err)
	}

	err = batch.Account(acctUrl).Main().Put(r1.Account)
	if err != nil {
		return errors.UnknownError.WithFormat("store account: %w", err)
	}

	// Read current chain heights
	r2, err := c.query.QueryAccountChains(ctx, acctUrl, nil)
	if err != nil {
		return errors.UnknownError.WithFormat("query account chains: %w", err)
	}

	starts := map[string]uint64{}
	total := map[string]uint64{}
	for _, state := range r2.Records {
		if !predicate(state) {
			continue
		}

		chain, err := batch.Account(acctUrl).ChainByName(state.Name)
		if err != nil {
			return errors.UnknownError.WithFormat("load %s chain: %w", state.Name, err)
		}

		head, err := chain.Head().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load %s chain head: %w", state.Name, err)
		}

		if head.Count < int64(state.Count) {
			starts[strings.ToLower(chain.Name())] = uint64(head.Count)
			total[strings.ToLower(chain.Name())] = state.Count
			continue
		}

		remote := &merkle.State{
			Count:   int64(state.Count),
			Pending: state.State,
		}
		if bytes.Equal(head.Anchor(), remote.Anchor()) {
			continue
		}

		err = c.identifyBadEntry(ctx, batch, chain, head, remote)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	// Commit account data
	err = batch.Commit()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	batch = c.OpenDB(true)
	defer func() { batch.Discard() }()

	// Update chains, 1000 entries at a time
	const N = 1000
	var i int
	for len(starts) > 0 {
		if i > 0 && i%20 == 0 {
			// Create a new batch
			err = batch.Commit()
			if err != nil {
				return err
			}
			batch = c.OpenDB(true)
		}
		i++

		// For each chain that needs to be updated
		var count uint64 = N
		var failed error
		for name, start := range starts {
			// Query the next 1000 entries
			slog.InfoCtx(ctx, "Pull chain entries", "account", acctUrl, "chain", name, "start", start, "of", total[name])
			r, err := c.query.QueryChainEntries(ctx, acctUrl, &api.ChainQuery{
				Name: name,
				Range: &api.RangeOptions{
					Start: start,
					Count: &count,
				},
			})
			if err != nil {
				return errors.UnknownError.WithFormat("load %s chain entries [%d, %d): %w", name, start, start+N, err)
			}

			// If there are no more entries...
			if start+uint64(len(r.Records)) >= r.Total {
				// Remove the chain from the map
				delete(starts, name)
			} else {
				// Otherwise update the start position
				starts[name] += uint64(len(r.Records))
			}

			chain, err := batch.Account(acctUrl).ChainByName(name)
			if err != nil {
				return errors.UnknownError.WithFormat("load %s chain: %w", name, err)
			}

			// For each entry
			for _, r := range r.Records {
				// Add it to the chain
				err = chain.Inner().AddHash(r.Entry[:], false)
				if err != nil {
					return errors.UnknownError.WithFormat("add entry to %s chain: %w", name, err)
				}
			}

			// Validate
			head, err := chain.Head().Get()
			if err != nil {
				return errors.UnknownError.WithFormat("load %s chain head: %w", name, err)
			}
			target := uint64(head.Count) - 1
			r2, err := c.query.QueryChainEntry(ctx, acctUrl, &api.ChainQuery{
				Name:  name,
				Index: &target,
			})
			if err != nil {
				return errors.UnknownError.WithFormat("query %s chain entry %d: %w", name, target, err)
			}
			remote := &merkle.State{
				Count:   head.Count,
				Pending: r2.State,
			}
			if !bytes.Equal(head.Anchor(), remote.Anchor()) {
				failed = errors.Conflict.WithFormat("checksum failed while pulling %v %v chain", acctUrl, name)
				slog.ErrorCtx(ctx, "Checksum failed while pulling chain", "account", acctUrl, "chain", name)
			}
		}
		if failed != nil {
			return failed
		}
	}

	err = batch.Commit()
	return errors.UnknownError.Wrap(err)
}

func (c *Client) identifyBadEntry(ctx context.Context, batch *DB, chain *database.Chain2, local, remote *merkle.State) error {
	wrong, ok := heightOfMismatch(local, remote)
	if !ok {
		panic("state does not match but cannot find the error")
	}
	if wrong == 0 {
		return errors.FatalError.WithFormat("%v %v chain entry %d is invalid", chain.Account(), chain.Name(), local.Count-1)
	}

	target := uint64(local.Count) & ^(1<<wrong) | ((1 << wrong) - 1) - 1
	local2, err := chain.State(int64(target))
	if err != nil {
		return errors.UnknownError.WithFormat("load %v %v chain state at %d: %w", chain.Account(), chain.Name(), target, err)
	}

	r, err := c.query.QueryChainEntry(ctx, chain.Account(), &api.ChainQuery{
		Name:  chain.Name(),
		Index: &target,
	})
	if err != nil {
		return errors.UnknownError.WithFormat("load %v %v chain state at %d: %w", chain.Account(), chain.Name(), target, err)
	}
	remote2 := &merkle.State{
		Count:   int64(target) + 1,
		Pending: r.State,
	}

	if local2.Count != remote2.Count {
		panic("height does not match")
	}

	if bytes.Equal(local2.Anchor(), remote2.Anchor()) {
		return errors.FatalError.WithFormat("%v %v chain entry %d is invalid", chain.Account(), chain.Name(), target+1)
	}

	return c.identifyBadEntry(ctx, batch, chain, local2, remote2)
}

func heightOfMismatch(a, b *merkle.State) (int, bool) {
	if a.Count != b.Count {
		panic("height does not match")
	}
	if len(a.Pending) != len(b.Pending) {
		panic("invalid state")
	}

	for i := len(a.Pending) - 1; i >= 0; i-- {
		a, b := a.Pending[i], b.Pending[i]
		switch {
		case bytes.Equal(a, b):
			continue
		case a == nil || b == nil:
			panic("invalid state")
		}
		return i, true
	}
	return 0, false
}

func (c *Client) PullPendingTransactionsForAccount(ctx context.Context, account *url.URL) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	batch := c.OpenDB(true)
	defer batch.Discard()

	// Get the resp list. It would be better to update this in the database,
	// but that's problematic since API v2 only returns the transaction hash,
	// not the ID.
	var resp struct {
		Items []string `json:"items"`
	}
	err := c.v2.RequestAPIv2(ctx, "query", &client.GeneralQuery{UrlQuery: client.UrlQuery{Url: account.WithFragment("pending")}}, &resp)
	if err != nil {
		return errors.UnknownError.WithFormat("query pending: %w", err)
	}

	missing := map[[32]byte]bool{}
	didLoad := batch.Index().Account(account).DidLoadTransaction()
	for _, s := range resp.Items {
		b, err := hex.DecodeString(s)
		if err != nil {
			return errors.UnknownError.WithFormat("query pending: invalid item: %w", err)
		}
		if len(b) != 32 {
			return errors.UnknownError.WithFormat("query pending: invalid item: not 32 bytes")
		}
		hash := *(*[32]byte)(b)

		// Check if it has already been fetched
		_, err = didLoad.Index(hash)
		switch {
		case err == nil:
			missing[hash] = false
		case errors.Is(err, errors.NotFound):
			// Continue
		default:
			return errors.UnknownError.WithFormat("load account did-load index: %w", err)
		}

		// Check if it has been fetched for a different account or chain
		_, err = batch.Message(hash).Main().Get()
		switch {
		case err == nil:
			missing[hash] = false
		case errors.Is(err, errors.NotFound):
			missing[hash] = true
			continue
		default:
			return errors.UnknownError.WithFormat("load transaction %x execution metadata: %w", b, err)
		}

		// Record that it has been fetched
		err = didLoad.Add(hash)
		if err != nil {
			return errors.UnknownError.WithFormat("store account did-load index: %w", err)
		}
	}

	// Build a slice of transaction IDs. Sorting this is not necessary but makes
	// the process less random and thus easier to debug.
	txids := make([]*url.TxID, 0, len(missing))
	for hash, missing := range missing {
		if missing {
			// We don't know the account ID so use unknown
			txids = append(txids, protocol.UnknownUrl().WithTxID(hash))
		}
	}
	sort.Slice(txids, func(i, j int) bool {
		a, b := txids[i], txids[j]
		return a.Compare(b) < 0
	})

	if len(txids) == 0 {
		return nil
	}

	// Pull the transactions
	slog.InfoCtx(ctx, "Pulling transactions", "count", len(txids), "account", account)
	err = c.pullTransactions(ctx, batch, txids)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Update the pending list. This is gross but there's not much we can do
	// until API v3 is live.
	pending := batch.Account(account).Pending()
	for hash := range missing {
		var msg *messaging.TransactionMessage
		err = batch.Message(hash).Main().GetAs(&msg)
		switch {
		case err == nil:
			err = pending.Add(msg.Transaction.ID())
		case errors.Is(err, errors.NotFound):
			continue // ☹
		default:
			return errors.UnknownError.Wrap(err)
		}
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	// This commits the main batch and the key-value store batch
	err = batch.Commit()
	return errors.UnknownError.Wrap(err)
}

type missingMessage struct {
	ID    *url.TxID
	Chain string
	Index uint64
}

func (c *Client) getMissingMessageIDs(batch *DB, account *url.URL, chains []string, since *time.Time) ([]*missingMessage, error) {
	var sinceMinor uint64
	if since != nil {
		if c.router == nil {
			return nil, errors.BadRequest.With("cannot determine major blocks without a router")
		}

		// Find the block
		part, err := c.router.RouteAccount(account)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("route %v: %w", account, err)
		}
		partU := protocol.PartitionUrl(part)
		_, block, err := batch.Index().Account(partU.JoinPath(protocol.Ledger)).Chain("root").Index().Find(ByIndexTime(*since))
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load anchor metadata: %w", err)
		}

		// Include chain entries since the given minor block
		sinceMinor = block.BlockIndex
	}

	// For each chain
	missing := map[[32]byte]bool{}
	data := map[[32]byte]*missingMessage{}
	didLoad := batch.Index().Account(account).DidLoadTransaction()
	for _, name := range chains {
		chain, err := batch.Account(account).ChainByName(name)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load %s chain: %w", name, err)
		}

		head, err := chain.Head().Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load main chain head: %w", err)
		}

		start := int64(0)
		if since != nil {
			_, entry, err := batch.Index().Account(account).Chain(name).Index().Find(ByIndexBlock(sinceMinor))
			if err != nil {
				return nil, errors.UnknownError.WithFormat("load %v %v chain index: %w", account, name, err)
			}
			start = int64(entry.Source)
		}

		// Scan all the entries
		const N = 1 << 10
		for i := start; i < head.Count; i += N {
			entries, err := chain.Inner().GetRange(i, i+N)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("load main chain entries [%d, %d): %w", i, i+N, err)
			}

			// For each entry
			for j, e := range entries {
				hash := *(*[32]byte)(e)
				data[hash] = &missingMessage{
					ID:    account.WithTxID(hash),
					Chain: chain.Name(),
					Index: uint64(i) + uint64(j),
				}
				if _, ok := missing[hash]; ok {
					continue
				}

				// Check if it has already been fetched
				_, err = didLoad.Index(hash)
				switch {
				case err == nil:
					missing[hash] = false
					continue
				case errors.Is(err, errors.NotFound):
					// Continue
				default:
					return nil, errors.UnknownError.WithFormat("load account did-load index: %w", err)
				}

				// Check if it has been fetched for a different account or chain
				_, err = batch.Message(hash).Main().Get()
				switch {
				case err == nil:
					missing[hash] = false
				case errors.Is(err, errors.NotFound):
					missing[hash] = true
					continue
				default:
					return nil, errors.UnknownError.WithFormat("load transaction %x execution metadata: %w", e, err)
				}

				// Record that it has been fetched
				err = didLoad.Add(hash)
				if err != nil {
					return nil, errors.UnknownError.WithFormat("store account did-load index: %w", err)
				}
			}
		}
	}

	// Build a slice of transaction IDs. Sorting this is not necessary but makes
	// the process less random and thus easier to debug.
	txids := make([]*missingMessage, 0, len(missing))
	for hash, missing := range missing {
		if missing {
			txids = append(txids, data[hash])
		}
	}
	sort.Slice(txids, func(i, j int) bool {
		a, b := txids[i], txids[j]
		c := strings.Compare(a.Chain, b.Chain)
		if c != 0 {
			return c < 0
		}
		return a.Index < b.Index
	})
	return txids, nil
}

// PullTransactionsForAccount fetches transactions from the account's chains.
func (c *Client) PullTransactionsForAccount(ctx context.Context, account *url.URL, chains ...string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	batch := c.OpenDB(true)
	defer batch.Discard()

	// Get missing transactions
	missing, err := c.getMissingMessageIDs(batch, account, chains, nil)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	if len(missing) == 0 {
		return nil
	}

	// Pull the transactions
	slog.InfoCtx(ctx, "Pulling transactions", "count", len(missing), "account", account)
	ids := make([]*url.TxID, len(missing))
	for i, m := range missing {
		ids[i] = m.ID
	}
	err = c.pullTransactions(ctx, batch, ids)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// This commits the main batch and the key-value store batch
	err = batch.Commit()
	return errors.UnknownError.Wrap(err)
}

// PullTransaction fetches a set of transactions.
func (c *Client) PullTransactions(ctx context.Context, txids ...*url.TxID) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	batch := c.OpenDB(true)
	defer batch.Discard()

	err := c.pullTransactions(ctx, batch, txids)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// This commits the main batch and the key-value store batch
	err = batch.Commit()
	return errors.UnknownError.Wrap(err)
}

func (c *Client) pullTransactions(ctx context.Context, batch *DB, txids []*url.TxID) error {
	// Create a JSON-RPC request for each
	var requests jsonrpc2.BatchRequest
	for _, id := range txids {
		params := &client.TxnQuery{
			IgnorePending: false,
			QueryOptions: client.QueryOptions{
				IncludeRemote: true,
			},
		}

		if protocol.IsUnknown(id.Account()) {
			h := id.Hash()
			params.Txid = h[:]
		} else {
			params.TxIdUrl = id
		}

		requests = append(requests, jsonrpc2.Request{
			ID:     1,
			Method: "query-tx",
			Params: params,
		})
	}

	// Fetch the transactions
	const N = 100
	total := len(requests)
	for i := 0; len(requests) > 0; i++ {
		// 100 at a time
		var req jsonrpc2.BatchRequest
		if len(requests) > N {
			req, requests = requests[:N], requests[N:]
		} else {
			req, requests = requests, nil
		}

		// Send the batch
		slog.InfoCtx(ctx, "Requesting transactions", "count", len(req), "batch-num", i+1, "batch-count", (total+N-1)/N)
		var responses []*client.TransactionQueryResponse
		err := c.v2.RequestAPIv2(ctx, "", req, &responses)
		if err != nil {
			return errors.UnknownError.WithFormat("query transactions: %w", err)
		}

		// Process the responses
		for _, r := range responses {
			err = batch.Message(r.Txid.Hash()).Main().Put(&messaging.TransactionMessage{Transaction: r.Transaction})
			if err != nil {
				return errors.UnknownError.WithFormat("store transaction %v (1): %w", r.Txid, err)
			}
			err = batch.Index().Account(r.Txid.Account()).DidLoadTransaction().Add(r.Txid.Hash())
			if err != nil {
				return errors.UnknownError.WithFormat("store transaction %v (2): %w", r.Txid, err)
			}
		}
	}

	return nil
}

func (c *Client) PullMessagesForAccount(ctx context.Context, account *url.URL, chains ...string) error {
	return c.pullMessagesForAccount(ctx, account, chains, nil)
}

func (c *Client) PullMessagesForAccountSince(ctx context.Context, account *url.URL, since time.Time, chains ...string) error {
	return c.pullMessagesForAccount(ctx, account, chains, &since)
}

func (c *Client) pullMessagesForAccount(ctx context.Context, account *url.URL, chains []string, since *time.Time) error {
	if c.query.Querier == nil {
		return errors.BadRequest.With("client was initialized without a querier")
	}

	batch := c.OpenDB(true)
	defer func() { batch.Discard() }()

	// Reuse connections
	ctx, cancel, _ := api.ContextWithBatchData(ctx)
	defer cancel()

	// Get missing messages
	slog.InfoCtx(ctx, "Checking for missing messages", "account", account, "chains", chains)
	missing, err := c.getMissingMessageIDs(batch, account, chains, since)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	if len(missing) == 0 {
		return nil
	}

	// Fetch
	slog.InfoCtx(ctx, "Fetching messages", "account", account, "chains", chains, "remaining", len(missing))
	didLoad := batch.Index().Account(account).DidLoadTransaction()
	for i, m := range missing {
		if i > 0 && i%100 == 0 {
			slog.InfoCtx(ctx, "Fetching messages", "account", account, "chains", chains, "remaining", len(missing)-i)
			err = batch.Commit()
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
			batch = c.OpenDB(true)
		}

		var msg messaging.Message
		if m.Chain == "" {
			r, err := c.query.QueryMessage(ctx, m.ID, nil)
			if err != nil {
				return errors.UnknownError.WithFormat("query %v: %w", m.ID, err)
			}
			msg = r.Message

		} else {
			r, err := c.query.QueryChainEntry(ctx, account, &api.ChainQuery{Name: m.Chain, Index: &m.Index})
			if err != nil {
				return errors.UnknownError.WithFormat("query %v: %w", m.ID, err)
			}
			mr, ok := r.Value.(*api.MessageRecord[messaging.Message])
			if !ok {
				if err, ok := r.Value.(*api.ErrorRecord); ok {
					slog.ErrorCtx(ctx, "Pull message", "account", account, "chain", m.Chain, "index", m.Index, "error", err.Value)
					continue
				} else {
					return errors.UnknownError.WithFormat("wanted %v, got %T", api.RecordTypeMessage, r.Value)
				}
			}
			msg = mr.Message
		}

		err = batch.Message(m.ID.Hash()).Main().Put(msg)
		if err != nil {
			return errors.UnknownError.WithFormat("store message %v (1): %w", m.ID, err)
		}
		err = didLoad.Add(m.ID.Hash())
		if err != nil {
			return errors.UnknownError.WithFormat("store message %v (2): %w", m.ID, err)
		}
	}

	err = batch.Commit()
	return errors.UnknownError.Wrap(err)
}

// IndexAccountChains collates the index chain entries for an account's chains.
func (c *Client) IndexAccountChains(ctx context.Context, acctUrl *url.URL) error {
	batch := c.OpenDB(true)
	defer batch.Discard()

	chains, err := batch.Account(acctUrl).Chains().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load chains index: %w", err)
	}

	// For each chain
	for _, x := range chains {
		if x.Type != merkle.ChainTypeIndex {
			continue
		}

		// Remove the -index suffix for chains like main-index so the index is
		// associated with the unsuffixed name (e.g. main). But there are also
		// index chains that don't have a suffix, such as major-block (currently
		// that's the only one but there could be others in the future).
		name := strings.TrimSuffix(x.Name, "-index")

		// Compare the chain head and index to identify new entries
		chain, err := batch.Account(acctUrl).ChainByName(x.Name)
		if err != nil {
			return errors.UnknownError.WithFormat("load %s index chain: %w", name, err)
		}

		head, err := chain.Head().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load %s index chain head: %w", name, err)
		}

		index, err := batch.Index().Account(acctUrl).Chain(name).Index().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load index of %s index chain: %w", name, err)
		}
		if len(index) >= int(head.Count) {
			continue
		}

		slog.InfoCtx(ctx, "Indexing an index chain", "account", acctUrl, "chain", name)

		// Load the new entries
		start := int64(len(index))
		entries, err := chain.Inner().GetRange(start, start+head.Count)
		if err != nil {
			return errors.UnknownError.WithFormat("load %s index chain entries [%d, %d): %w", name, start, start+head.Count, err)
		}

		// Add them to the index
		index = nil
		for _, b := range entries {
			entry := new(protocol.IndexEntry)
			err = entry.UnmarshalBinary(b)
			if err != nil {
				return errors.UnknownError.WithFormat("unmarshal %s index chain entry: %w", name, err)
			}

			index = append(index, entry)
		}

		err = batch.Index().Account(acctUrl).Chain(name).Index().Add(index...)
		if err != nil {
			return errors.UnknownError.WithFormat("store index of %s index chain: %w", name, err)
		}
	}

	err = batch.Commit()
	return errors.UnknownError.Wrap(err)
}

// IndexProducedAnchors collates a partition's produced block anchors.
func (c *Client) IndexProducedAnchors(ctx context.Context, partUrl *url.URL) error {
	batch := c.OpenDB(true)
	defer batch.Discard()

	// Compare the anchor sequence chain head to the index to identify new
	// entries
	chain := batch.Account(partUrl.JoinPath(protocol.AnchorPool)).AnchorSequenceChain()
	head, err := chain.Head().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load %v anchor sequence chain head: %w", partUrl, err)
	}

	anchors, err := batch.Index().Partition(partUrl).Anchors().Produced().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load %v anchor index: %w", partUrl, err)
	}
	if len(anchors) >= int(head.Count) {
		return nil
	}

	slog.InfoCtx(ctx, "Indexing produced anchors", "account", partUrl)

	// Load the new entries
	start := int64(len(anchors))
	entries, err := chain.Inner().GetRange(start, start+head.Count)
	if err != nil {
		return errors.UnknownError.WithFormat("load %v anchor sequence chain entries [%d, %d): %w", partUrl, start, start+head.Count, err)
	}

	// For each entry
	anchors = nil
	for i, e := range entries {
		// Load the transaction
		hash := *(*[32]byte)(e)
		var msg *messaging.TransactionMessage
		err := batch.Message(hash).Main().GetAs(&msg)
		if err != nil {
			return errors.UnknownError.WithFormat("load %v anchor %d: %w", partUrl, start+int64(i), err)
		}
		anchor, ok := msg.Transaction.Body.(protocol.AnchorBody)
		if !ok {
			return errors.UnknownError.WithFormat("%v anchor %d is invalid: expected anchor, got %T", partUrl, start+int64(i), msg.Transaction.Body)
		}

		// Add it to the index
		anchors = append(anchors, &AnchorMetadata{
			Index:       uint64(start) + uint64(i),
			Hash:        hash,
			Anchor:      anchor.GetPartitionAnchor(),
			Transaction: msg.Transaction,
		})
	}

	err = batch.Index().Partition(partUrl).Anchors().Produced().Add(anchors...)
	if err != nil {
		return errors.UnknownError.WithFormat("store index of %v anchor sequence chain: %w", partUrl, err)
	}

	err = batch.Commit()
	return errors.UnknownError.Wrap(err)
}

// IndexReceivedAnchors collates a partition's received block anchors.
func (c *Client) IndexReceivedAnchors(ctx context.Context, partUrl *url.URL) error {
	batch := c.OpenDB(true)
	defer batch.Discard()

	// Compare the chain head to the index to identify new entries
	chain := batch.Account(partUrl.JoinPath(protocol.AnchorPool)).MainChain()
	head, err := chain.Head().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load %v anchor sequence chain head: %w", partUrl, err)
	}

	anchors, err := batch.Index().Partition(partUrl).Anchors().Received().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load %v anchor index: %w", partUrl, err)
	}
	if len(anchors) >= int(head.Count) {
		return nil
	}

	slog.InfoCtx(ctx, "Indexing received anchors", "account", partUrl)

	// Find the first missing entry
	var start int64
	have := map[uint64]bool{}
	for _, a := range anchors {
		have[a.Index] = true
		if a.Index == uint64(start) {
			start++
		}
	}

	// Load the new entries
	entries, err := chain.Inner().GetRange(start, head.Count)
	if err != nil {
		return errors.UnknownError.WithFormat("load %v anchor sequence chain entries [%d, %d): %w", partUrl, start, start+head.Count, err)
	}

	// For each entry
	var notFound int
	rdBatch := c.OpenDB(false)
	defer func() { rdBatch.Discard() }()
	for i, e := range entries {
		// Skip existing
		if have[uint64(start)+uint64(i)] {
			continue
		}

		// Load the transaction
		hash := *(*[32]byte)(e)
		var msg *messaging.TransactionMessage
		err := rdBatch.Message(hash).Main().GetAs(&msg)
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				slog.DebugCtx(ctx, "Anchor not found", "partition", partUrl, "hash", logging.AsHex(hash), "height", i)
				notFound++
				continue
			}
			return errors.UnknownError.WithFormat("load %v anchor %d: %w", partUrl, int64(i), err)
		}
		anchor, ok := msg.Transaction.Body.(protocol.AnchorBody)
		if !ok {
			continue
		}

		// Limit how much is held in the cache
		if (i+1)%1000 == 0 {
			rdBatch.Discard()
			rdBatch = c.OpenDB(false)
		}

		// Add it to the index
		anchors = append(anchors, &AnchorMetadata{
			Index:       uint64(start) + uint64(i),
			Hash:        hash,
			Anchor:      anchor.GetPartitionAnchor(),
			Transaction: msg.Transaction,
		})
	}
	if notFound > 0 {
		slog.ErrorCtx(ctx, "Missing received anchors", "partition", partUrl, "count", notFound)
	}

	sort.Slice(anchors, func(i, j int) bool {
		a, b := anchors[i], anchors[j]
		return a.Index < b.Index
	})

	err = batch.Index().Partition(partUrl).Anchors().Received().Put(anchors)
	if err != nil {
		return errors.UnknownError.WithFormat("store index of %v anchor sequence chain: %w", partUrl, err)
	}

	err = batch.Commit()
	return errors.UnknownError.Wrap(err)
}

// IndexAccountTransaction indexes the local and directory block index and time
// for the accounts' main chain transactions.
func (c *Client) IndexAccountTransactions(ctx context.Context, accounts ...*url.URL) error {
	if c.router == nil {
		return errors.BadRequest.With("cannot index without a router")
	}

	batch := c.OpenDB(true)
	defer batch.Discard()

	// Load the directory's root chain index
	dirRootIndex, err := batch.Index().Account(protocol.DnUrl().JoinPath(protocol.Ledger)).Chain("root").Index().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load directory root chain index: %w", err)
	}

	// Load the root chain and anchor index of each partition, and the
	// directory's anchor chain index for each partition
	type PartData struct {
		ID                        string
		RootIndex, DirAnchorIndex []*protocol.IndexEntry
		AnchorIndex               []*AnchorMetadata
	}
	parts := map[string]*PartData{}
	routes := map[*url.URL]string{}
	for _, account := range accounts {
		// For each account... route the account
		partId, err := c.router.RouteAccount(account)
		if err != nil {
			return errors.UnknownError.WithFormat("route account: %w", err)
		}
		routes[account] = partId

		// Check if we've already loaded the indices for that account
		if _, ok := parts[partId]; ok {
			continue
		}

		part := new(PartData)
		part.ID = partId
		parts[partId] = part

		// Load the partition's root chain index
		part.RootIndex, err = batch.Index().Account(protocol.PartitionUrl(partId).JoinPath(protocol.Ledger)).Chain("root").Index().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load root chain index: %w", err)
		}

		// Load the partition's anchor index
		part.AnchorIndex, err = batch.Index().Partition(protocol.PartitionUrl(partId)).Anchors().Produced().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load anchor index: %w", err)
		}

		// Load the directory's anchor chain index for the partition
		name := batch.Account(protocol.DnUrl().JoinPath(protocol.AnchorPool)).AnchorChain(partId).Root().Name()
		part.DirAnchorIndex, err = batch.Index().Account(protocol.DnUrl().JoinPath(protocol.AnchorPool)).Chain(name).Index().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load directory anchor ledger main chain index: %w", err)
		}
	}

	// For each account
	for _, account := range accounts {
		// Load the main chain index
		mainIndex, err := batch.Index().Account(account).Chain("main").Index().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load main chain index: %w", err)
		}

		// Process all of the main chain entries
		head, err := batch.Account(account).MainChain().Head().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load main chain head: %w", err)
		}

		// In batches of 1024
		const N = 1 << 10
		part := parts[routes[account]]
		didIndex := batch.Index().Account(account).DidIndexTransactionExecution()
		var didLog bool
		for i := int64(0); i < head.Count; i += N {
			entries, err := batch.Account(account).MainChain().Inner().GetRange(i, i+N)
			if err != nil {
				return errors.UnknownError.WithFormat("load main chain entries [%d, %d): %w", i, i+N, err)
			}

			// For each entry
			for j, e := range entries {
				// Check if it has already been indexed
				hash := *(*[32]byte)(e)
				_, err = didIndex.Index(hash)
				switch {
				case err == nil:
					continue
				case errors.Is(err, errors.NotFound):
					// Ok
				default:
					return errors.UnknownError.WithFormat("load transaction %x execution metadata: %w", e, err)
				}

				// Record that it has been indexed
				err = didIndex.Add(hash)
				if err != nil {
					return errors.UnknownError.WithFormat("update did index index: %w", err)
				}

				if !didLog {
					slog.InfoCtx(ctx, "Indexing transactions", "account", account)
					didLog = true
				}

				// Get main chain index entry (source, anchor, block index)
				_, main, ok := FindEntry(mainIndex, ByIndexSource(uint64(i)+uint64(j)))
				if !ok {
					return errors.NotFound.WithFormat("cannot find index entry for main chain entry %d", i+int64(j))
				}

				// Get root chain index entry (block index, block time, source)
				_, root, ok := FindEntry(part.RootIndex, ByIndexSource(main.Anchor))
				if !ok {
					return errors.NotFound.WithFormat("cannot find index entry for root chain entry %d", main.Source)
				}

				// Get anchor
				_, anchor, ok := FindEntry(part.AnchorIndex, ByAnchorBlock(root.BlockIndex))
				if !ok || anchor.Anchor.MinorBlockIndex != root.BlockIndex {
					return errors.NotFound.WithFormat("cannot find anchor for %s block %d", part.ID, root.BlockIndex)
				}

				// TODO skip if not anchored

				// Find the index when the anchor was anchored
				dirAnchorPos, err := batch.Account(protocol.DnUrl().JoinPath(protocol.AnchorPool)).AnchorChain(routes[account]).Root().IndexOf(anchor.Anchor.RootChainAnchor[:])
				if err != nil {
					return errors.UnknownError.WithFormat("locate chain entry for block %d anchor: %w", root.BlockIndex, err)
				}

				// Get directory anchor chain index entry (source, anchor, block index)
				_, dirAnchor, ok := FindEntry(part.DirAnchorIndex, ByIndexSource(uint64(dirAnchorPos)))
				if !ok {
					return errors.NotFound.WithFormat("cannot find index entry for directory anchor chain entry %d", dirAnchorPos)
				}

				// Get directory root chain index entry (block index, block time, source)
				_, dirRoot, ok := FindEntry(dirRootIndex, ByIndexSource(dirAnchor.Anchor))
				if !ok {
					return errors.NotFound.WithFormat("cannot find index entry for directory root chain entry %d", dirAnchor.Anchor)
				}

				err = batch.Index().Transaction(hash).Executed().Put(&EventMetadata{
					LocalBlock:     root.BlockIndex,
					LocalTime:      *root.BlockTime,
					DirectoryBlock: dirRoot.BlockIndex,
					DirectoryTime:  *dirRoot.BlockTime,
				})
				if err != nil {
					return errors.UnknownError.WithFormat("store transaction %x execution metadata: %w", e, err)
				}
			}
		}
	}

	err = batch.Commit()
	return errors.UnknownError.Wrap(err)
}

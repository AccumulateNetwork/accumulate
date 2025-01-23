// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package light

import (
	"bytes"
	"context"
	"encoding/hex"
	stderr "errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slices"
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
			slog.InfoContext(ctx, "Pull chain entries", "account", acctUrl, "chain", name, "start", start, "of", total[name])
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
				err = chain.Inner().AddEntry(r.Entry[:], false)
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
				slog.ErrorContext(ctx, "Checksum failed while pulling chain", "account", acctUrl, "chain", name)
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
	for _, s := range resp.Items {
		b, err := hex.DecodeString(s)
		if err != nil {
			return errors.UnknownError.WithFormat("query pending: invalid item: %w", err)
		}
		if len(b) != 32 {
			return errors.UnknownError.WithFormat("query pending: invalid item: not 32 bytes")
		}
		hash := *(*[32]byte)(b)

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
	slog.InfoContext(ctx, "Pulling transactions", "count", len(txids), "account", account)
	err = c.PullTransactions(ctx, txids...)
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

func (c *Client) GetBlockForTime(account *url.URL, time time.Time) (uint64, error) {
	batch := c.OpenDB(false)
	defer batch.Discard()
	return c.getBlockForTime(batch, account, time)
}

func (c *Client) getBlockForTime(batch *DB, account *url.URL, time time.Time) (uint64, error) {
	if c.router == nil {
		return 0, errors.BadRequest.With("cannot determine major blocks without a router")
	}

	// Find the block
	part, err := c.router.RouteAccount(account)
	if err != nil {
		return 0, errors.UnknownError.WithFormat("route %v: %w", account, err)
	}
	partU := protocol.PartitionUrl(part)
	block, err := batch.Index().Account(partU.JoinPath(protocol.Ledger)).Chain("root").BlockTime().FindIndexEntryBefore(time)
	switch {
	case err == nil:
		// Include chain entries since the given minor block
		return block.BlockIndex, nil

	case errors.Is(err, errors.NotFound):
		// If there's no entry before the given time, start with block 1
		return 1, nil

	default:
		return 0, errors.UnknownError.WithFormat("load anchor metadata: %w", err)
	}
}

type missingMessage struct {
	ID    *url.TxID
	Chain string
	Index uint64
}

// PullTransactionsForAccount fetches transactions from the account's chains.
//
// DEPRECATED: Use PullMessagesForAccount.
func (c *Client) PullTransactionsForAccount(ctx context.Context, account *url.URL, chains ...string) error {
	return c.PullMessagesForAccount(ctx, account, chains...)
}

// PullTransaction fetches a set of transactions.
func (c *Client) PullTransactions(ctx context.Context, txids ...*url.TxID) error {
	var missing []*missingMessage
	for _, id := range txids {
		missing = append(missing, &missingMessage{ID: id})
	}

	err := c.pullMessages(ctx, missing)
	return errors.UnknownError.Wrap(err)
}

func (c *Client) PullMessagesForAccount(ctx context.Context, account *url.URL, chains ...string) error {
	return c.pullMessagesForAccount(ctx, account, chains, nil)
}

func (c *Client) PullMessagesForAccountSince(ctx context.Context, account *url.URL, since time.Time, chains ...string) error {
	since = since.UTC() // For debugging
	return c.pullMessagesForAccount(ctx, account, chains, &since)
}

func (c *Client) pullMessagesForAccount(ctx context.Context, account *url.URL, chains []string, since *time.Time) error {
	slog.InfoContext(ctx, "Checking for missing messages", "account", account, "chains", chains)

	if c.query.Querier == nil {
		return errors.BadRequest.With("client was initialized without a querier")
	}

	batch := c.OpenDB(false)
	defer batch.Discard()

	// Reuse connections
	ctx, cancel, _ := api.ContextWithBatchData(ctx)
	defer cancel()

	var sinceMinor uint64
	var err error
	if since != nil {
		// Include chain entries since the given minor block
		sinceMinor, err = c.getBlockForTime(batch, account, *since)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}

	slog.InfoContext(ctx, "Checking for missing messages", "account", account, "chains", chains)
	var missing []*missingMessage
	for _, name := range chains {
		chain, err := batch.Account(account).ChainByName(name)
		if err != nil {
			return errors.UnknownError.WithFormat("load %s chain: %w", name, err)
		}

		head, err := chain.Head().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load main chain head: %w", err)
		}

		start := int64(0)
		if since != nil {
			entry, err := batch.Index().Account(account).Chain(name).BlockIndex().FindIndexEntryAfter(sinceMinor)
			switch {
			case err == nil:
				start = int64(entry.Source)
			case errors.Is(err, errors.NotFound):
				// If there's no entry after the given block, there must not have been anything written to the chain since then
				continue
			default:
				return errors.UnknownError.WithFormat("load %v %v chain index: %w", account, name, err)
			}
		}

		// Scan all the entries
		const N = 1 << 10
		didLoad := batch.Index().Account(account).Chain(name).DidLoad()
		for i := start; i < head.Count; i += N {
			entries, err := chain.Inner().Entries(i, i+N)
			if err != nil {
				return errors.UnknownError.WithFormat("load main chain entries [%d, %d): %w", i, i+N, err)
			}

			// For each entry
			for j, e := range entries {
				index := uint64(i) + uint64(j)
				ok, err := didLoad.Contains(index)
				if err != nil {
					return errors.UnknownError.Wrap(err)
				}
				if ok {
					continue
				}

				hash := *(*[32]byte)(e)
				missing = append(missing, &missingMessage{
					ID:    account.WithTxID(hash),
					Chain: chain.Name(),
					Index: uint64(i) + uint64(j),
				})
			}
		}
	}

	return c.pullMessages(ctx, missing)
}

func (c *Client) pullMessages(ctx context.Context, missing []*missingMessage) error {
	if len(missing) == 0 {
		return nil
	}

	if c.query.Querier == nil {
		return errors.BadRequest.With("client was initialized without a querier")
	}

	batch := c.OpenDB(true)
	defer func() { batch.Discard() }()

	// Reuse connections
	ctx, cancel, _ := api.ContextWithBatchData(ctx)
	defer cancel()

	// For logging
	var accounts []*url.URL
	for _, m := range missing {
		u := m.ID.Account()
		if m.Chain != "" {
			u = u.WithUserInfo(m.Chain)
		}
		i, ok := slices.BinarySearchFunc(accounts, u, (*url.URL).Compare)
		if !ok {
			accounts = slices.Insert(accounts, i, u)
		}
	}

	slog.InfoContext(ctx, "Fetching messages", "chains", accounts, "remaining", len(missing))
	for i, m := range missing {
		if i > 0 && i%100 == 0 {
			err := batch.Commit()
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
			batch = c.OpenDB(true)
			slog.InfoContext(ctx, "Fetching messages", "chains", accounts, "remaining", len(missing)-i)
		}

		// Verify the message is actually missing
		_, err := batch.Message(m.ID.Hash()).Main().Get()
		switch {
		case err == nil:
			// Don't fetch it

		case !errors.Is(err, errors.NotFound):
			return errors.UnknownError.WithFormat("load message %v: %w", m.ID, err)

		default:
			// Get the message
			var message messaging.Message
			var err error
			if m.Chain == "" {
				r, err := c.query.QueryMessage(ctx, m.ID, nil)
				if err != nil {
					return errors.UnknownError.WithFormat("query %v: %w", m.ID, err)
				}
				message = r.Message

			} else {
				r, err := c.query.QueryMainChainEntry(ctx, m.ID.Account(), &api.ChainQuery{Name: m.Chain, Index: &m.Index})
				if err != nil {
					return errors.UnknownError.WithFormat("query %v: %w", m.ID, err)
				}
				message = r.Value.Message
			}

			// Record it
			err = batch.Message(m.ID.Hash()).Main().Put(message)
			if err != nil {
				return errors.UnknownError.WithFormat("store message %v (1): %w", m.ID, err)
			}
		}

		// Record that the chain entry has been loaded
		if m.Chain != "" {
			_, err = batch.Index().Account(m.ID.Account()).Chain(m.Chain).DidLoad().Add(m.Index)
			if err != nil {
				return errors.UnknownError.WithFormat("store message %v (2): %w", m.ID, err)
			}
		}
	}

	err := batch.Commit()
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
		slog.InfoContext(ctx, "Indexing an index chain", "account", acctUrl, "chain", name)

		// Load the index chain
		chain, err := batch.Account(acctUrl).ChainByName(x.Name)
		if err != nil {
			return errors.UnknownError.WithFormat("load %s index chain: %w", name, err)
		}

		// Update the source index
		indices := batch.Index().Account(acctUrl).Chain(name)
		err = buildIndexChainIndex(chain.Inner(), indices.SourceIndex(), indexSource)
		if err != nil {
			return errors.UnknownError.WithFormat("update source index of %s index chain: %w", name, err)
		}

		// Update the root index index
		err = buildIndexChainIndex(chain.Inner(), indices.RootIndexIndex(), indexRootIndexIndex)
		if err != nil {
			return errors.UnknownError.WithFormat("update root index index of %s index chain: %w", name, err)
		}

		// Update the block index
		err = buildIndexChainIndex(chain.Inner(), indices.BlockIndex(), indexBlockIndex)
		if err != nil {
			return errors.UnknownError.WithFormat("update block index of %s index chain: %w", name, err)
		}

		// Update the block time index
		if x.Name == "root-index" {
			err = buildIndexChainIndex(chain.Inner(), indices.BlockTime(), indexBlockTime)
			if err != nil {
				return errors.UnknownError.WithFormat("update block time index of %s index chain: %w", name, err)
			}
		}
	}

	err = batch.Commit()
	return errors.UnknownError.Wrap(err)
}

// buildIndexChainIndex updates a chain index for an index chain
func buildIndexChainIndex[T any](chain *merkle.Chain, index *ChainIndex[T], value func(*protocol.IndexEntry) (T, error)) error {
	return buildChainIndex(chain, index, func(b []byte) (T, error) {
		entry := new(protocol.IndexEntry)
		err := entry.UnmarshalBinary(b)
		if err != nil {
			var z T
			return z, errors.UnknownError.WithFormat("unmarshal chain entry: %w", err)
		}
		return value(entry)
	})
}

func indexSource(e *protocol.IndexEntry) (uint64, error) {
	return e.Source, nil
}

func indexBlockIndex(e *protocol.IndexEntry) (uint64, error) {
	return e.BlockIndex, nil
}

func indexBlockTime(e *protocol.IndexEntry) (time.Time, error) {
	if e.BlockTime == nil {
		return time.Time{}, errSkip
	}
	return *e.BlockTime, nil
}

func indexRootIndexIndex(e *protocol.IndexEntry) (uint64, error) {
	return e.RootIndexIndex, nil
}

func buildMessageIndex[M messaging.Message, T any](batch *DB, chain *merkle.Chain, index *ChainIndex[T], value func(M) (T, error)) error {
	return buildChainIndex(chain, index, func(b []byte) (T, error) {
		var msg M
		var z T
		err := batch.Message2(b).Main().GetAs(&msg)
		switch {
		case err == nil:
			return value(msg)
		case errors.Is(err, errors.NotFound):
			return z, errSkip // Skip missing messages
		default:
			return z, errors.UnknownError.WithFormat("load message: %w", err)
		}
	})
}

var errSkip = stderr.New("skip")

func buildChainIndex[T any](chain *merkle.Chain, index *ChainIndex[T], value func([]byte) (T, error)) error {
	// Get the chain head
	head, err := chain.Head().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load chain head: %w", err)
	}

	// Get the last indexed height
	_, lastIndexed, err := index.Last()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.UnknownError.WithFormat("load chain head: %w", err)
	}
	if lastIndexed >= uint64(head.Count) {
		return nil
	}

	// Load the new chain entries
	start := int64(lastIndexed)
	entries, err := chain.Entries(start, start+head.Count)
	if err != nil {
		return errors.UnknownError.WithFormat("load chain entries [%d, %d): %w", start, start+head.Count, err)
	}

	// Add them to the index
	for i, b := range entries {
		v, err := value(b)
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, errSkip):
			continue
		default:
			return errors.UnknownError.WithFormat("chain entry %d: %w", i+int(start), err)
		}

		err = index.Append(v, uint64(start)+uint64(i))
		if err != nil {
			return errors.UnknownError.WithFormat("update source index: %w", err)
		}
	}
	return nil
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

	slog.InfoContext(ctx, "Indexing produced anchors", "account", partUrl)

	// Load the new entries
	start := int64(len(anchors))
	entries, err := chain.Inner().Entries(start, start+head.Count)
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
	defer func() { batch.Discard() }()

	// Load globals
	g := new(core.GlobalValues)
	err := g.LoadNetwork(protocol.DnUrl(), func(accountUrl *url.URL, target interface{}) error {
		return batch.Account(accountUrl).Main().GetAs(target)
	})
	if err != nil {
		return errors.UnknownError.WithFormat("load globals: %w", err)
	}

	slog.InfoContext(ctx, "Indexing received anchors", "account", partUrl)

	// Index by DN block
	chain := batch.Account(partUrl.JoinPath(protocol.AnchorPool)).MainChain()
	indices := batch.Index().Partition(partUrl).Anchors()
	err = buildMessageIndex(batch, chain.Inner(), indices.Received(protocol.DnUrl()), func(msg *messaging.TransactionMessage) (uint64, error) {
		body, ok := msg.Transaction.Body.(*protocol.DirectoryAnchor)
		if !ok {
			return 0, errSkip
		}
		return body.MinorBlockIndex, nil
	})
	if err != nil {
		return errors.UnknownError.WithFormat("update DN anchor block index: %w", err)
	}

	// Index by BVN block
	for _, part := range g.Network.Partitions {
		if part.Type != protocol.PartitionTypeBlockValidator {
			continue
		}
		source := protocol.PartitionUrl(part.ID)
		err = buildMessageIndex(batch, chain.Inner(), indices.Received(source), func(msg *messaging.TransactionMessage) (uint64, error) {
			body, ok := msg.Transaction.Body.(*protocol.DirectoryAnchor)
			if !ok {
				return 0, errSkip
			}
			var block uint64
			for _, r := range body.Receipts {
				if r.Anchor.Source.Equal(source) && r.Anchor.MinorBlockIndex > block {
					block = r.Anchor.MinorBlockIndex
				}
			}
			if block == 0 {
				return 0, errSkip
			}
			return block, nil
		})
		if err != nil {
			return errors.UnknownError.WithFormat("update %s anchor block index: %w", part.ID, err)
		}
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
	dirRootIndex := batch.Index().Account(protocol.DnUrl().JoinPath(protocol.Ledger)).Chain("root")

	// Load the root chain and anchor index of each partition, and the
	// directory's anchor chain index for each partition
	type PartData struct {
		ID                        string
		Url                       *url.URL
		RootIndex, DirAnchorIndex *IndexDBAccountChain
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
		part.Url = protocol.PartitionUrl(partId)
		parts[partId] = part

		// Load the partition's root chain index
		part.RootIndex = batch.Index().Account(protocol.PartitionUrl(partId).JoinPath(protocol.Ledger)).Chain("root")

		// Load the partition's anchor index
		part.AnchorIndex, err = batch.Index().Partition(protocol.PartitionUrl(partId)).Anchors().Produced().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load anchor index: %w", err)
		}

		// Load the directory's anchor chain index for the partition
		name := batch.Account(protocol.DnUrl().JoinPath(protocol.AnchorPool)).AnchorChain(partId).Root().Name()
		part.DirAnchorIndex = batch.Index().Account(protocol.DnUrl().JoinPath(protocol.AnchorPool)).Chain(name)
	}

	// For each account
	for _, account := range accounts {
		// Load the main chain index
		mainIndex := batch.Index().Account(account).Chain("main")

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
			entries, err := batch.Account(account).MainChain().Inner().Entries(i, i+N)
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
					slog.InfoContext(ctx, "Indexing transactions", "account", account)
					didLog = true
				}

				// Get main chain index entry (source, anchor, block index)
				main, err := mainIndex.SourceIndex().FindIndexEntryAfter(uint64(i) + uint64(j))
				if err != nil {
					return errors.NotFound.WithFormat("cannot find index entry for main chain entry %d: %w", i+int64(j), err)
				}

				// Get root chain index entry (block index, block time, source)
				root, err := part.RootIndex.SourceIndex().FindIndexEntryAfter(main.Anchor)
				if err != nil {
					return errors.NotFound.WithFormat("cannot find index entry for root chain entry %d: %w", main.Source, err)
				}

				// Get root chain state
				state, err := batch.Account(part.Url.JoinPath(protocol.Ledger)).RootChain().State(int64(root.Source))
				if err != nil {
					return errors.NotFound.WithFormat("load %s root chain state at %d: %w", part.ID, root.Source, err)
				}

				// TODO skip if not anchored

				// Find the index when the anchor was anchored
				dirAnchorPos, err := batch.Account(protocol.DnUrl().JoinPath(protocol.AnchorPool)).AnchorChain(routes[account]).Root().IndexOf(state.Anchor())
				if err != nil {
					return errors.UnknownError.WithFormat("locate chain entry for block %d anchor: %w", root.BlockIndex, err)
				}

				// Get directory anchor chain index entry (source, anchor, block index)
				dirAnchor, err := part.DirAnchorIndex.SourceIndex().FindIndexEntryAfter(uint64(dirAnchorPos))
				if err != nil {
					return errors.NotFound.WithFormat("cannot find index entry for directory anchor chain entry %d: %w", dirAnchorPos, err)
				}

				// Get directory root chain index entry (block index, block time, source)
				dirRoot, err := dirRootIndex.SourceIndex().FindIndexEntryAfter(dirAnchor.Anchor)
				if err != nil {
					return errors.NotFound.WithFormat("cannot find index entry for directory root chain entry %d: %w", dirAnchor.Anchor, err)
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

	err := batch.Commit()
	return errors.UnknownError.Wrap(err)
}

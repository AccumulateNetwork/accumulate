// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package light

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/AccumulateNetwork/jsonrpc2/v15"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// PullAccount fetches the latest version of the account and its chains.
func (c *Client) PullAccount(ctx context.Context, acctUrl *url.URL) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	batch := c.OpenDB(true)
	defer batch.Discard()

	// Update the account
	var raw json.RawMessage
	chains, err := c.v2.QueryAccountAs(ctx, &client.GeneralQuery{UrlQuery: client.UrlQuery{Url: acctUrl}}, &raw)
	if err != nil {
		var jerr jsonrpc2.Error
		if errors.As(err, &jerr) && jerr.Code == client.ErrCodeNotFound {
			return errors.NotFound.Wrap(err)
		}
		return errors.UnknownError.WithFormat("query account: %w", err)
	}

	account, err := protocol.UnmarshalAccountJSON(raw)
	if err != nil {
		return errors.UnknownError.WithFormat("unmarshal request: %w", err)
	}

	err = batch.Account(acctUrl).Main().Put(account)
	if err != nil {
		return errors.UnknownError.WithFormat("store account: %w", err)
	}

	// Read current chain heights
	starts := map[string]uint64{}
	for _, state := range chains.Chains {
		c, err := batch.Account(acctUrl).ChainByName(state.Name)
		if err != nil {
			return errors.UnknownError.WithFormat("load %s chain: %w", state.Name, err)
		}

		head, err := c.Head().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load %s chain head: %w", state.Name, err)
		}

		if head.Count < int64(state.Height) {
			starts[strings.ToLower(c.Name())] = uint64(head.Count)
		}
	}

	// Update chains, 1000 entries at a time
	const N = 1000
	first := true
	for len(starts) > 0 {
		// Commit and reopen the batch to ensure Badger doesn't explode
		if !first {
			err = batch.Commit()
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
			batch = c.OpenDB(true)
			defer batch.Discard()
		}
		first = false

		// For each chain that needs to be updated
		for name, start := range starts {
			// Query the next 1000 entries
			var res *client.MultiResponse
			u := acctUrl.WithFragment(fmt.Sprintf("chain/%s", name)).WithQuery(fmt.Sprintf("start=%d&count=%d", start, N))
			c.logger.Debug("Request", "url", u)
			err = c.v2.RequestAPIv2(ctx, "query", &client.GeneralQuery{UrlQuery: client.UrlQuery{Url: u}}, &res)
			if err != nil {
				return errors.UnknownError.WithFormat("load %s chain entries [%d, %d): %w", name, start, start+N, err)
			}

			// If there are no more entries...
			if start+uint64(len(res.Items)) >= res.Total {
				// Remove the chain from the map
				delete(starts, name)
			} else {
				// Otherwise update the start position
				starts[name] += uint64(len(res.Items))
			}

			chain, err := batch.Account(acctUrl).ChainByName(name)
			if err != nil {
				return errors.UnknownError.WithFormat("load %s chain: %w", name, err)
			}

			// For each entry
			for _, raw := range res.Items {
				b, err := json.Marshal(raw)
				if err != nil {
					return errors.InternalError.WithFormat("unable to remarshal response: %w", err)
				}

				// Unmarshal the entry; API v2 has a bug, so index entries have
				// to be unmarshalled and remarshalled
				if chain.Type() == merkle.ChainTypeIndex {
					var entry *protocol.IndexEntry
					err = json.Unmarshal(b, &entry)
					if err != nil {
						return errors.UnknownError.WithFormat("unmarshal response: %w", err)
					}

					b, err = entry.MarshalBinary()
					if err != nil {
						return errors.InternalError.WithFormat("unable to remarshal response: %w", err)
					}

					if len(b) < 32 {
						padding := make([]byte, 32-len(b))
						// Fake field number to make unmarshalling work
						padding[0] = 32
						b = append(b, padding...)
					}

				} else {
					var res *client.ChainQueryResponse
					err = json.Unmarshal(b, &res)
					if err != nil {
						return errors.UnknownError.WithFormat("unmarshal response: %w", err)
					}

					str, ok := res.Data.(string)
					if !ok {
						return errors.UnknownError.WithFormat("invalid chain response: want %T, got %T", "", res.Data)
					}
					b, err = hex.DecodeString(str)
					if err != nil {
						return errors.UnknownError.WithFormat("unmarshal response: %w", err)
					}
				}

				// Add it to the chain
				err = chain.Inner().AddHash(b, false)
				if err != nil {
					return errors.UnknownError.WithFormat("add entry to %s chain: %w", name, err)
				}
			}
		}
	}

	err = batch.Commit()
	return errors.UnknownError.Wrap(err)
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
	c.logger.Info("Pulling transactions", "count", len(txids), "account", account)
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

// PullTransactionsForAccount fetches transactions from the account's chains.
func (c *Client) PullTransactionsForAccount(ctx context.Context, account *url.URL, chains ...string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	batch := c.OpenDB(true)
	defer batch.Discard()

	// For each chain
	missing := map[[32]byte]bool{}
	didLoad := batch.Index().Account(account).DidLoadTransaction()
	for _, name := range chains {
		chain, err := batch.Account(account).ChainByName(name)
		if err != nil {
			return errors.UnknownError.WithFormat("load %s chain: %w", name, err)
		}

		head, err := chain.Head().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load main chain head: %w", err)
		}

		// Scan all the entries
		const N = 1 << 10
		for i := int64(0); i < head.Count; i += N {
			entries, err := chain.Inner().GetRange(i, i+N)
			if err != nil {
				return errors.UnknownError.WithFormat("load main chain entries [%d, %d): %w", i, i+N, err)
			}

			// For each entry
			for _, e := range entries {
				hash := *(*[32]byte)(e)
				if _, ok := missing[hash]; ok {
					continue
				}

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
				default:
					return errors.UnknownError.WithFormat("load transaction %x execution metadata: %w", e, err)
				}

				// Record that it has been fetched
				err = didLoad.Add(hash)
				if err != nil {
					return errors.UnknownError.WithFormat("store account did-load index: %w", err)
				}
			}
		}
	}

	// Build a slice of transaction IDs. Sorting this is not necessary but makes
	// the process less random and thus easier to debug.
	txids := make([]*url.TxID, 0, len(missing))
	for hash, missing := range missing {
		if missing {
			txids = append(txids, account.WithTxID(hash))
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
	c.logger.Info("Pulling transactions", "count", len(txids), "account", account)
	err := c.pullTransactions(ctx, batch, txids)
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
		c.logger.Info("Requesting transactions", "count", len(req), "batch-num", i+1, "batch-count", (total+N-1)/N)
		var responses []*client.TransactionQueryResponse
		err := c.v2.RequestAPIv2(ctx, "", req, &responses)
		if err != nil {
			return errors.UnknownError.WithFormat("query transactions: %w", err)
		}

		// Process the responses
		for _, r := range responses {
			err = batch.Message(r.Txid.Hash()).Main().Put(&messaging.TransactionMessage{Transaction: r.Transaction})
			if err != nil {
				return errors.UnknownError.WithFormat("store transaction %v: %w", r.Txid, err)
			}
		}
	}

	return nil
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

		c.logger.Info("Indexing an index chain", "account", acctUrl, "chain", name)

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

// IndexAnchors collates a partition's block anchors.
func (c *Client) IndexAnchors(ctx context.Context, partUrl *url.URL) error {
	batch := c.OpenDB(true)
	defer batch.Discard()

	// Compare the anchor sequence chain head to the index to identify new
	// entries
	chain := batch.Account(partUrl.JoinPath(protocol.AnchorPool)).AnchorSequenceChain()
	head, err := chain.Head().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load %v anchor sequence chain head: %w", partUrl, err)
	}

	anchors, err := batch.Index().Partition(partUrl).Anchors().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load %v anchor index: %w", partUrl, err)
	}
	if len(anchors) >= int(head.Count) {
		return nil
	}

	c.logger.Info("Indexing anchors", "account", partUrl)

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
			Index:  uint64(start) + uint64(i),
			Hash:   hash,
			Anchor: anchor.GetPartitionAnchor(),
		})
	}

	err = batch.Index().Partition(partUrl).Anchors().Add(anchors...)
	if err != nil {
		return errors.UnknownError.WithFormat("store index of %v anchor sequence chain: %w", partUrl, err)
	}

	err = batch.Commit()
	return errors.UnknownError.Wrap(err)
}

// IndexAccountTransaction indexes the local and directory block index and time
// for the accounts' main chain transactions.
func (c *Client) IndexAccountTransactions(ctx context.Context, accounts ...*url.URL) error {
	batch := c.OpenDB(true)
	defer batch.Discard()

	// Get a router to determine which partition each account belongs to
	router, err := c.router(batch)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Load the directory's root chain index
	dirRootIndex, err := batch.Index().Account(protocol.DnUrl().JoinPath(protocol.Ledger)).Chain("root").Index().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load directory root chain index: %w", err)
	}

	// Load the root chain and anchor index of each partition, and the
	// directory's anchor chain index for each partition
	type PartData struct {
		RootIndex, DirAnchorIndex []*protocol.IndexEntry
		AnchorIndex               []*AnchorMetadata
	}
	parts := map[string]*PartData{}
	routes := map[*url.URL]string{}
	for _, account := range accounts {
		// For each account... route the account
		partId, err := router.RouteAccount(account)
		if err != nil {
			return errors.UnknownError.WithFormat("route account: %w", err)
		}
		routes[account] = partId

		// Check if we've already loaded the indices for that account
		if _, ok := parts[partId]; ok {
			continue
		}

		part := new(PartData)
		parts[partId] = part

		// Load the partition's root chain index
		part.RootIndex, err = batch.Index().Account(protocol.PartitionUrl(partId).JoinPath(protocol.Ledger)).Chain("root").Index().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load root chain index: %w", err)
		}

		// Load the partition's anchor index
		part.AnchorIndex, err = batch.Index().Partition(protocol.PartitionUrl(partId)).Anchors().Get()
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
					c.logger.Info("Indexing transactions", "account", account)
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
					return errors.NotFound.WithFormat("cannot find anchor for block %d", root.BlockIndex)
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

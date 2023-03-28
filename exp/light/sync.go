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

	// Update chains
	const N = 1000
	transactions := map[[32]byte]struct{}{}
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

		for name, start := range starts {
			chain, err := batch.Account(acctUrl).ChainByName(name)
			if err != nil {
				return errors.UnknownError.WithFormat("load %s chain: %w", name, err)
			}

			var res *client.MultiResponse
			u := acctUrl.WithFragment(fmt.Sprintf("chain/%s", name)).WithQuery(fmt.Sprintf("start=%d&count=%d", start, N))
			fmt.Printf("Request %v\n", u)
			err = c.v2.RequestAPIv2(ctx, "query", &client.GeneralQuery{UrlQuery: client.UrlQuery{Url: u}}, &res)
			if err != nil {
				return errors.UnknownError.WithFormat("load %s chain entries [%d, %d): %w", name, start, start+N, err)
			}

			if start+uint64(len(res.Items)) >= res.Total {
				delete(starts, name)
			} else {
				starts[name] += uint64(len(res.Items))
			}

			for _, raw := range res.Items {
				b, err := json.Marshal(raw)
				if err != nil {
					return errors.InternalError.WithFormat("unable to remarshal response: %w", err)
				}

				// API v2 has a bug, so index entries have to be unmarshalled and remarshalled
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

				err = chain.Inner().AddHash(b, false)
				if err != nil {
					return errors.UnknownError.WithFormat("add entry to %s chain: %w", name, err)
				}

				if chain.Type() != merkle.ChainTypeTransaction || name == "signature" {
					continue
				}

				_, err = batch.Message2(b).Main().Get()
				switch {
				case err == nil:
					continue // Already have the transaction
				case errors.Is(err, errors.NotFound):
					transactions[*(*[32]byte)(b)] = struct{}{}
				default:
					return errors.UnknownError.WithFormat("load transaction %x: %w", b, err)
				}
			}
		}
	}

	err = batch.Commit()
	return errors.UnknownError.Wrap(err)
}

func (c *Client) PullTransactionsForAccount(ctx context.Context, account *url.URL, chains ...string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	batch := c.OpenDB(true)
	defer batch.Discard()

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

		const N = 1 << 10
		for i := int64(0); i < head.Count; i += N {
			entries, err := chain.Inner().GetRange(i, i+N)
			if err != nil {
				return errors.UnknownError.WithFormat("load main chain entries [%d, %d): %w", i, i+N, err)
			}

			for _, e := range entries {
				hash := *(*[32]byte)(e)
				if _, ok := missing[hash]; ok {
					continue
				}

				_, err = didLoad.Index(hash)
				switch {
				case err == nil:
					missing[hash] = false
				case errors.Is(err, errors.NotFound):
					// Continue
				default:
					return errors.UnknownError.WithFormat("load account did-load index: %w", err)
				}

				_, err = batch.Message(hash).Main().Get()
				switch {
				case err == nil:
					missing[hash] = false
				case errors.Is(err, errors.NotFound):
					missing[hash] = true
				default:
					return errors.UnknownError.WithFormat("load transaction %x execution metadata: %w", e, err)
				}

				err = didLoad.Add(hash)
				if err != nil {
					return errors.UnknownError.WithFormat("store account did-load index: %w", err)
				}
			}
		}
	}

	// Build a slice of transaction IDs. Sorting this is not necessary but makes
	// the process less random.
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
	fmt.Printf("Pulling %d transactions for %v\n", len(txids), account)
	return c.PullTransactions(ctx, txids...)
}

func (c *Client) PullTransactions(ctx context.Context, txids ...*url.TxID) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	batch := c.OpenDB(true)
	defer batch.Discard()

	var requests jsonrpc2.BatchRequest
	for _, id := range txids {
		requests = append(requests, jsonrpc2.Request{
			ID:     1,
			Method: "query-tx",
			Params: &client.TxnQuery{
				TxIdUrl:       id,
				IgnorePending: false,
				QueryOptions: client.QueryOptions{
					IncludeRemote: true,
				},
			},
		})
	}

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

		fmt.Printf("Requesting %d (batch %d of %d) transaction(s)\n", len(req), i+1, (total+N-1)/N)
		var responses []*client.TransactionQueryResponse
		err := c.v2.RequestAPIv2(ctx, "", req, &responses)
		if err != nil {
			return errors.UnknownError.WithFormat("query transactions: %w", err)
		}

		for _, r := range responses {
			err = batch.Message(r.Txid.Hash()).Main().Put(&messaging.TransactionMessage{Transaction: r.Transaction})
			if err != nil {
				return errors.UnknownError.WithFormat("store transaction %v: %w", r.Txid, err)
			}
		}
	}

	// This commits the main batch and the key-value store batch
	err := batch.Commit()
	return errors.UnknownError.Wrap(err)
}

func (c *Client) IndexAccountChains(ctx context.Context, acctUrl *url.URL) error {
	batch := c.OpenDB(true)
	defer batch.Discard()

	chains, err := batch.Account(acctUrl).Chains().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load chains index: %w", err)
	}

	for _, c := range chains {
		if strings.HasSuffix(c.Name, "-index") {
			continue
		}

		chain, err := batch.Account(acctUrl).ChainByName(c.Name)
		if err != nil {
			return errors.UnknownError.WithFormat("load %s index chain: %w", c.Name, err)
		}

		if c.Type != merkle.ChainTypeIndex {
			chain = chain.Index()
		}

		head, err := chain.Head().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load %s index chain head: %w", c.Name, err)
		}

		index, err := batch.Index().Account(acctUrl).Chain(c.Name).Index().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load index of %s index chain: %w", c.Name, err)
		}
		if len(index) >= int(head.Count) {
			continue
		}

		fmt.Printf("Indexing %v %s index chain\n", acctUrl, c.Name)

		start := int64(len(index))
		entries, err := chain.Inner().GetRange(start, start+head.Count)
		if err != nil {
			return errors.UnknownError.WithFormat("load %s index chain entries [%d, %d): %w", c.Name, start, start+head.Count, err)
		}

		index = nil
		for _, b := range entries {
			entry := new(protocol.IndexEntry)
			err = entry.UnmarshalBinary(b)
			if err != nil {
				return errors.UnknownError.WithFormat("unmarshal %s index chain entry: %w", c.Name, err)
			}

			index = append(index, entry)
		}

		err = batch.Index().Account(acctUrl).Chain(c.Name).Index().Add(index...)
		if err != nil {
			return errors.UnknownError.WithFormat("store index of %s index chain: %w", c.Name, err)
		}
	}

	err = batch.Commit()
	return errors.UnknownError.Wrap(err)
}

func (c *Client) IndexAnchors(ctx context.Context, partUrl *url.URL) error {
	batch := c.OpenDB(true)
	defer batch.Discard()

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

	fmt.Printf("Indexing %v anchors\n", partUrl)

	start := int64(len(anchors))
	entries, err := chain.Inner().GetRange(start, start+head.Count)
	if err != nil {
		return errors.UnknownError.WithFormat("load %v anchor sequence chain entries [%d, %d): %w", partUrl, start, start+head.Count, err)
	}

	anchors = nil
	for i, e := range entries {
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

func (c *Client) IndexAccountTransactions(ctx context.Context, accounts ...*url.URL) error {
	batch := c.OpenDB(true)
	defer batch.Discard()

	router, err := c.router(batch)
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	dirRootIndex, err := batch.Index().Account(protocol.DnUrl().JoinPath(protocol.Ledger)).Chain("root").Index().Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load directory root chain index: %w", err)
	}

	type PartData struct {
		RootIndex, DirAnchorIndex []*protocol.IndexEntry
		AnchorIndex               []*AnchorMetadata
	}
	parts := map[string]*PartData{}
	routes := map[*url.URL]string{}
	for _, account := range accounts {
		partId, err := router.RouteAccount(account)
		if err != nil {
			return errors.UnknownError.WithFormat("route account: %w", err)
		}
		routes[account] = partId
		if _, ok := parts[partId]; ok {
			continue
		}

		part := new(PartData)
		parts[partId] = part

		part.RootIndex, err = batch.Index().Account(protocol.PartitionUrl(partId).JoinPath(protocol.Ledger)).Chain("root").Index().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load root chain index: %w", err)
		}

		part.AnchorIndex, err = batch.Index().Partition(protocol.PartitionUrl(partId)).Anchors().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load anchor index: %w", err)
		}

		name := batch.Account(protocol.DnUrl().JoinPath(protocol.AnchorPool)).AnchorChain(partId).Root().Name()
		part.DirAnchorIndex, err = batch.Index().Account(protocol.DnUrl().JoinPath(protocol.AnchorPool)).Chain(name).Index().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load directory anchor ledger main chain index: %w", err)
		}
	}
	(*protocol.TransactionStatus)(nil).Failed()


	for _, account := range accounts {
		mainIndex, err := batch.Index().Account(account).Chain("main").Index().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load main chain index: %w", err)
		}

		head, err := batch.Account(account).MainChain().Head().Get()
		if err != nil {
			return errors.UnknownError.WithFormat("load main chain head: %w", err)
		}

		const N = 1 << 10
		part := parts[routes[account]]
		didIndex := batch.Index().Account(account).DidIndexTransactionExecution()
		var didLog bool
		for i := int64(0); i < head.Count; i += N {
			entries, err := batch.Account(account).MainChain().Inner().GetRange(i, i+N)
			if err != nil {
				return errors.UnknownError.WithFormat("load main chain entries [%d, %d): %w", i, i+N, err)
			}

			for j, e := range entries {
				hash := *(*[32]byte)(e)
				_, err = didIndex.Index(hash)
				switch {
				case err == nil:
					continue
				case errors.Is(err, errors.NotFound):
					// Build index
				default:
					return errors.UnknownError.WithFormat("load transaction %x execution metadata: %w", e, err)
				}
				err = didIndex.Add(hash)
				if err != nil {
					return errors.UnknownError.WithFormat("update did index index: %w", err)
				}
				if !didLog {
					fmt.Printf("Indexing %v's transactions\n", account)
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

// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const defaultPageSize = 50
const maxPageSize = 100

type Querier struct {
	logger    logging.OptionalLogger
	db        database.Viewer
	partition config.NetworkUrl
	consensus api.ConsensusService
}

var _ api.Querier = (*Querier)(nil)

type QuerierParams struct {
	Logger    log.Logger
	Database  database.Viewer
	Partition string
	Consensus api.ConsensusService
}

func NewQuerier(params QuerierParams) *Querier {
	s := new(Querier)
	s.logger.L = params.Logger
	s.db = params.Database
	s.consensus = params.Consensus
	s.partition.URL = protocol.PartitionUrl(params.Partition)
	return s
}

func (s *Querier) Type() api.ServiceType { return api.ServiceTypeQuery }

func (s *Querier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	// Ensure the query parameters are valid
	if query == nil {
		query = new(api.DefaultQuery)
	}
	err := query.IsValid()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("query is invalid: %w", err)
	}

	switch query := query.(type) {
	case *api.ChainQuery:
		fixRange(query.Range)
	case *api.DataQuery:
		fixRange(query.Range)
	case *api.DirectoryQuery:
		fixRange(query.Range)
	case *api.PendingQuery:
		fixRange(query.Range)
	case *api.BlockQuery:
		fixRange(query.MinorRange)
		fixRange(query.MajorRange)
		fixRange(query.EntryRange)
	}

	// Start a batch. If the ABCI were updated to commit in the middle of a
	// block, this would no longer be safe.
	var r api.Record
	err = s.db.View(func(batch *database.Batch) error {
		r, err = s.query(ctx, batch, scope, query)
		return err
	})
	return r, err
}

func (s *Querier) getLastBlockTime(ctx context.Context, batch *database.Batch) *time.Time {
	if s.consensus != nil {
		var no = false
		cs, err := s.consensus.ConsensusStatus(ctx, api.ConsensusStatusOptions{
			IncludePeers:      &no,
			IncludeAccumulate: &no,
		})
		if err == nil {
			return &cs.LastBlock.Time
		}
	}

	var ledger *protocol.SystemLedger
	if batch.Account(s.partition.Ledger()).Main().GetAs(&ledger) == nil {
		return &ledger.Timestamp
	}

	// Cannot determine last block time
	return nil
}

func (s *Querier) query(ctx context.Context, batch *database.Batch, scope *url.URL, query api.Query) (api.Record, error) {
	switch query := query.(type) {
	case *api.DefaultQuery:
		if txid, err := scope.AsTxID(); err == nil {
			r, err := s.queryMessage(ctx, batch, txid)
			if r != nil {
				r.LastBlockTime = s.getLastBlockTime(ctx, batch)
			}
			return r, err
		}

		r, err := s.queryAccount(ctx, batch, batch.Account(scope), query.IncludeReceipt)
		if r != nil {
			r.LastBlockTime = s.getLastBlockTime(ctx, batch)
		}
		return r, err

	case *api.ChainQuery:
		if query.Name == "" {
			if txid, err := scope.AsTxID(); err == nil {
				h := txid.Hash()
				r, err := s.queryTransactionChains(ctx, batch, h[:], query.IncludeReceipt)
				if r != nil {
					r.LastBlockTime = s.getLastBlockTime(ctx, batch)
				}
				return r, err
			}
			r, err := s.queryAccountChains(ctx, batch.Account(scope))
			if r != nil {
				r.LastBlockTime = s.getLastBlockTime(ctx, batch)
			}
			return r, err
		}

		record, err := batch.Account(scope).ChainByName(query.Name)
		if err != nil {
			return nil, errors.InternalError.WithFormat("get chain %s: %w", query.Name, err)
		}

		if query.Index != nil {
			r, err := s.queryChainEntryByIndex(ctx, batch, record, *query.Index, true, query.IncludeReceipt)
			if r != nil {
				r.LastBlockTime = s.getLastBlockTime(ctx, batch)
			}
			return r, err
		}

		if query.Entry != nil {
			r, err := s.queryChainEntryByValue(ctx, batch, record, query.Entry, true, query.IncludeReceipt)
			if r != nil {
				r.LastBlockTime = s.getLastBlockTime(ctx, batch)
			}
			return r, err
		}

		if query.Range != nil {
			r, err := s.queryChainEntryRange(ctx, batch, record, query.Range)
			if r != nil {
				r.LastBlockTime = s.getLastBlockTime(ctx, batch)
			}
			return r, err
		}

		r, err := s.queryChain(ctx, record)
		if r != nil {
			r.LastBlockTime = s.getLastBlockTime(ctx, batch)
		}
		return r, err

	case *api.DataQuery:
		if query.Index != nil {
			r, err := s.queryDataEntryByIndex(ctx, batch, indexing.Data(batch, scope), *query.Index, true)
			if r != nil {
				r.LastBlockTime = s.getLastBlockTime(ctx, batch)
			}
			return r, err
		}

		if query.Entry != nil {
			r, err := s.queryDataEntryByHash(ctx, batch, indexing.Data(batch, scope), query.Entry, true)
			if r != nil {
				r.LastBlockTime = s.getLastBlockTime(ctx, batch)
			}
			return r, err
		}

		if query.Range != nil {
			r, err := s.queryDataEntryRange(ctx, batch, indexing.Data(batch, scope), query.Range)
			if r != nil {
				r.LastBlockTime = s.getLastBlockTime(ctx, batch)
			}
			return r, err
		}

		r, err := s.queryLastDataEntry(ctx, batch, indexing.Data(batch, scope))
		if r != nil {
			r.LastBlockTime = s.getLastBlockTime(ctx, batch)
		}
		return r, err

	case *api.DirectoryQuery:
		r, err := s.queryDirectoryRange(ctx, batch, batch.Account(scope), query.Range)
		if r != nil {
			r.LastBlockTime = s.getLastBlockTime(ctx, batch)
		}
		return r, err

	case *api.PendingQuery:
		r, err := s.queryPendingRange(ctx, batch, batch.Account(scope), query.Range)
		if r != nil {
			r.LastBlockTime = s.getLastBlockTime(ctx, batch)
		}
		return r, err

	case *api.BlockQuery:
		if query.Minor != nil {
			r, err := s.queryMinorBlock(ctx, batch, *query.Minor, query.EntryRange)
			if r != nil {
				r.LastBlockTime = s.getLastBlockTime(ctx, batch)
			}
			return r, err
		}

		if query.Major != nil {
			r, err := s.queryMajorBlock(ctx, batch, *query.Major, query.MinorRange, query.OmitEmpty)
			if r != nil {
				r.LastBlockTime = s.getLastBlockTime(ctx, batch)
			}
			return r, err
		}

		if query.MinorRange != nil {
			r, err := s.queryMinorBlockRange(ctx, batch, query.MinorRange, query.OmitEmpty)
			if r != nil {
				r.LastBlockTime = s.getLastBlockTime(ctx, batch)
			}
			return r, err
		}

		r, err := s.queryMajorBlockRange(ctx, batch, query.MajorRange, query.OmitEmpty)
		if r != nil {
			r.LastBlockTime = s.getLastBlockTime(ctx, batch)
		}
		return r, err

	case *api.AnchorSearchQuery:
		r, err := s.searchForAnchor(ctx, batch, batch.Account(scope), query.Anchor, query.IncludeReceipt)
		if r != nil {
			r.LastBlockTime = s.getLastBlockTime(ctx, batch)
		}
		return r, err

	case *api.PublicKeySearchQuery:
		hash, err := protocol.PublicKeyHash(query.PublicKey, query.Type)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("hash key: %w", err)
		}
		r, err := s.searchForKeyEntry(ctx, batch, scope, func(s protocol.Signer) (int, protocol.KeyEntry, bool) {
			return s.EntryByKeyHash(hash)
		})
		if r != nil {
			r.LastBlockTime = s.getLastBlockTime(ctx, batch)
		}
		return r, err

	case *api.PublicKeyHashSearchQuery:
		r, err := s.searchForKeyEntry(ctx, batch, scope, func(s protocol.Signer) (int, protocol.KeyEntry, bool) {
			return s.EntryByKeyHash(query.PublicKeyHash)
		})
		if r != nil {
			r.LastBlockTime = s.getLastBlockTime(ctx, batch)
		}
		return r, err

	case *api.DelegateSearchQuery:
		r, err := s.searchForKeyEntry(ctx, batch, scope, func(s protocol.Signer) (int, protocol.KeyEntry, bool) {
			return s.EntryByDelegate(query.Delegate)
		})
		if r != nil {
			r.LastBlockTime = s.getLastBlockTime(ctx, batch)
		}
		return r, err

	case *api.MessageHashSearchQuery:
		r, err := s.searchForTransactionHash(ctx, batch, query.Hash)
		if r != nil {
			r.LastBlockTime = s.getLastBlockTime(ctx, batch)
		}
		return r, err

	default:
		return nil, errors.NotAllowed.WithFormat("unknown query type %v", query.QueryType())
	}
}

func (s *Querier) queryAccount(ctx context.Context, batch *database.Batch, record *database.Account, wantReceipt bool) (*api.AccountRecord, error) {
	r := new(api.AccountRecord)

	state, err := record.Main().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load state: %w", err)
	}
	r.Account = state

	switch state.Type() {
	case protocol.AccountTypeIdentity, protocol.AccountTypeKeyBook:
		directory, err := record.Directory().Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load directory: %w", err)
		}
		r.Directory, _ = api.MakeRange(directory, 0, defaultPageSize, func(v *url.URL) (*api.UrlRecord, error) {
			return &api.UrlRecord{Value: v}, nil
		})
		r.Directory.Total = uint64(len(directory))
	}

	pending, err := record.Pending().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load pending: %w", err)
	}
	r.Pending, _ = api.MakeRange(pending, 0, defaultPageSize, func(v *url.TxID) (*api.TxIDRecord, error) {
		return &api.TxIDRecord{Value: v}, nil
	})
	r.Pending.Total = uint64(len(pending))

	if !wantReceipt {
		return r, nil
	}

	block, receipt, err := indexing.ReceiptForAccountState(s.partition, batch, record)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get state receipt: %w", err)
	}

	r.Receipt = new(api.Receipt)
	r.Receipt.Receipt = *receipt
	r.Receipt.LocalBlock = block.BlockIndex
	if block.BlockTime != nil {
		r.Receipt.LocalBlockTime = *block.BlockTime
	}
	return r, nil
}

func (s *Querier) queryMessage(ctx context.Context, batch *database.Batch, txid *url.TxID) (*api.MessageRecord[messaging.Message], error) {
	return loadMessage(batch, txid)
}

func (s *Querier) queryTransaction(ctx context.Context, batch *database.Batch, txid *url.TxID) (*api.MessageRecord[*messaging.TransactionMessage], error) {
	r, err := s.queryMessage(ctx, batch, txid)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	r2, err := api.MessageRecordAs[*messaging.TransactionMessage](r)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("%v is not a transaction: %w", txid, err)
	}
	return r2, nil
}

func (s *Querier) queryAccountChains(ctx context.Context, record *database.Account) (*api.RecordRange[*api.ChainRecord], error) {
	chains, err := record.Chains().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load chains index: %w", err)
	}

	r := new(api.RecordRange[*api.ChainRecord])
	r.Total = uint64(len(chains))
	r.Records = make([]*api.ChainRecord, len(chains))
	for i, c := range chains {
		cr, err := s.queryChainByName(ctx, record, c.Name)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("chain %s: %w", c.Name, err)
		}

		r.Records[i] = cr
	}
	return r, nil
}

func (s *Querier) queryTransactionChains(ctx context.Context, batch *database.Batch, hash []byte, wantReceipt bool) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	entries, err := batch.Transaction(hash).Chains().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load transaction chains: %w", err)
	}

	return api.MakeRange(entries, 0, 0, func(e *database.TransactionChainEntry) (*api.ChainEntryRecord[api.Record], error) {
		c, err := batch.Account(e.Account).ChainByName(e.Chain)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load %v %s chain: %w", e.Account, e.Chain, err)
		}

		// We could try to be smart and use e.ChainIndex to find the entry, but
		// that is the index of an index chain entry (**not** the index of the
		// transaction's entry on the chain), and on top of that if there were
		// multiple transactions in the block, the index entry may not point to
		// this transaction's entry.
		r, err := s.queryChainEntryByValue(ctx, batch, c, hash, false, wantReceipt)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

		if r.Receipt != nil && !bytes.Equal(r.Receipt.Start, hash) {
			return nil, errors.InternalError.With("receipt does not belong to the transaction")
		}

		r.Account = e.Account
		return r, nil
	})
}

func (s *Querier) queryChainByName(ctx context.Context, record *database.Account, name string) (*api.ChainRecord, error) {
	chain, err := record.ChainByName(name)
	if err != nil {
		return nil, errors.InternalError.WithFormat("get chain %s: %w", name, err)
	}

	return s.queryChain(ctx, chain)
}

func (s *Querier) queryChain(ctx context.Context, record *database.Chain2) (*api.ChainRecord, error) {
	head, err := record.Head().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load head: %w", err)
	}

	r := new(api.ChainRecord)
	r.Name = record.Name()
	r.Type = record.Type()
	r.Count = uint64(head.Count)
	r.State = head.Pending
	return r, nil
}

func (s *Querier) queryChainEntryByIndex(ctx context.Context, batch *database.Batch, record *database.Chain2, index uint64, expand, wantReceipt bool) (*api.ChainEntryRecord[api.Record], error) {
	value, err := record.Entry(int64(index))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get entry: %w", err)
	}
	return s.queryChainEntry(ctx, batch, record, index, value, expand, wantReceipt)
}

func (s *Querier) queryChainEntryByValue(ctx context.Context, batch *database.Batch, record *database.Chain2, value []byte, expand, wantReceipt bool) (*api.ChainEntryRecord[api.Record], error) {
	index, err := record.IndexOf(value)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get entry index: %w", err)
	}
	return s.queryChainEntry(ctx, batch, record, uint64(index), value, expand, wantReceipt)
}

func (s *Querier) queryChainEntry(ctx context.Context, batch *database.Batch, record *database.Chain2, index uint64, value []byte, expand, wantReceipt bool) (*api.ChainEntryRecord[api.Record], error) {
	r := new(api.ChainEntryRecord[api.Record])
	r.Name = record.Name()
	r.Type = record.Type()
	r.Index = index
	r.Entry = *(*[32]byte)(value)

	ms, err := record.Inner().StateAt(int64(index))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load state: %w", err)
	}
	r.State = ms.Pending

	if expand {
		// The data model incorrectly labels synthetic sequence chains as
		// transaction chains when in reality they are index chains
		typ := r.Type
		if strings.HasPrefix(r.Name, "synthetic-sequence(") {
			typ = merkle.ChainTypeIndex
		}

		switch typ {
		case merkle.ChainTypeIndex:
			v := new(protocol.IndexEntry)
			err := v.UnmarshalBinary(value)
			if err == nil {
				r.Value = &api.IndexEntryRecord{Value: v}
			} else {
				r.Value = &api.ErrorRecord{
					Value: errors.UnknownError.Wrap(err).(*errors.Error),
				}
			}

		case merkle.ChainTypeTransaction:
			var err error
			r.Value, err = s.queryMessage(ctx, batch, protocol.UnknownUrl().WithTxID(r.Entry))
			switch {
			case err == nil:
				// Ok
			case errors.Is(err, errors.NotFound):
				r.Value = &api.ErrorRecord{
					Value: errors.UnknownError.Wrap(err).(*errors.Error),
				}
			default:
				return nil, errors.UnknownError.Wrap(err)
			}
		}
	}

	if wantReceipt {
		block, rootIndexIndex, receipt, err := indexing.ReceiptForChainIndex(s.partition, batch, record, int64(index))
		if err != nil {
			return nil, errors.UnknownError.WithFormat("get chain receipt: %w", err)
		}

		r.Receipt = new(api.Receipt)
		r.Receipt.Receipt = *receipt
		r.Receipt.LocalBlock = block.BlockIndex
		if block.BlockTime != nil {
			r.Receipt.LocalBlockTime = *block.BlockTime
		}

		// Find the major block
		rxc, err := batch.Account(s.partition.AnchorPool()).MajorBlockChain().Get()
		if err != nil {
			return nil, errors.InternalError.WithFormat("load root index chain: %w", err)
		}
		_, major, err := indexing.SearchIndexChain(rxc, 0, indexing.MatchAfter, indexing.SearchIndexChainByRootIndexIndex(rootIndexIndex))
		switch {
		case err == nil:
			r.Receipt.MajorBlock = major.BlockIndex
		case errors.Is(err, errors.NotFound):
			// Not in a major block yet
		default:
			return nil, errors.InternalError.WithFormat("locate major block for root index entry %d: %w", rootIndexIndex, err)
		}
	}

	return r, nil
}

func (s *Querier) queryDataEntryByIndex(ctx context.Context, batch *database.Batch, record *indexing.DataIndexer, index uint64, expand bool) (*api.ChainEntryRecord[*api.MessageRecord[*messaging.TransactionMessage]], error) {
	entryHash, err := record.Entry(index)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get entry hash: %w", err)
	}

	return s.queryDataEntry(ctx, batch, record, index, entryHash, expand)
}

func (s *Querier) queryLastDataEntry(ctx context.Context, batch *database.Batch, record *indexing.DataIndexer) (*api.ChainEntryRecord[*api.MessageRecord[*messaging.TransactionMessage]], error) {
	count, err := record.Count()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get count: %w", err)
	}
	if count == 0 {
		return nil, errors.NotFound.WithFormat("account has no data entries")
	}
	return s.queryDataEntryByIndex(ctx, batch, record, count-1, true)
}

func (s *Querier) queryDataEntryByHash(ctx context.Context, batch *database.Batch, record *indexing.DataIndexer, entryHash []byte, expand bool) (*api.ChainEntryRecord[*api.MessageRecord[*messaging.TransactionMessage]], error) {
	// TODO: Find a way to get the index without scanning the entire set
	return s.queryDataEntry(ctx, batch, record, 0, entryHash, expand)
}

func (s *Querier) queryDataEntry(ctx context.Context, batch *database.Batch, record *indexing.DataIndexer, index uint64, entryHash []byte, expand bool) (*api.ChainEntryRecord[*api.MessageRecord[*messaging.TransactionMessage]], error) {
	txnHash, err := record.Transaction(entryHash)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get transaction hash: %w", err)
	}

	r := new(api.ChainEntryRecord[*api.MessageRecord[*messaging.TransactionMessage]])
	r.Name = "data"
	r.Type = protocol.ChainTypeTransaction
	r.Index = index
	r.Entry = *(*[32]byte)(entryHash)

	if expand {
		r.Value, err = s.queryTransaction(ctx, batch, protocol.UnknownUrl().WithTxID(*(*[32]byte)(txnHash)))
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load transaction: %w", err)
		}
	}

	return r, nil
}

func (s *Querier) queryChainEntryRange(ctx context.Context, batch *database.Batch, record *database.Chain2, opts *api.RangeOptions) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	head, err := record.Head().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get head: %w", err)
	}

	r := new(api.RecordRange[*api.ChainEntryRecord[api.Record]])
	r.Total = uint64(head.Count)
	allocRange(r, opts, zeroBased)

	for i := range r.Records {
		var expand bool
		if opts.Expand != nil {
			expand = *opts.Expand
		}
		r.Records[i], err = s.queryChainEntryByIndex(ctx, batch, record, r.Start+uint64(i), expand, false)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("get entry %d: %w", r.Start+uint64(i), err)
		}
	}
	return r, nil
}

func (s *Querier) queryDataEntryRange(ctx context.Context, batch *database.Batch, record *indexing.DataIndexer, opts *api.RangeOptions) (*api.RecordRange[*api.ChainEntryRecord[*api.MessageRecord[*messaging.TransactionMessage]]], error) {
	total, err := record.Count()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get count: %w", err)
	}

	r := new(api.RecordRange[*api.ChainEntryRecord[*api.MessageRecord[*messaging.TransactionMessage]]])
	r.Total = total
	allocRange(r, opts, zeroBased)

	expand := true
	if opts.Expand != nil {
		expand = *opts.Expand
	}
	for i := range r.Records {
		r.Records[i], err = s.queryDataEntryByIndex(ctx, batch, record, r.Start+uint64(i), expand)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("get entry %d: %w", r.Start+uint64(i), err)
		}
	}
	return r, nil
}

func (s *Querier) queryDirectoryRange(ctx context.Context, batch *database.Batch, record *database.Account, opts *api.RangeOptions) (*api.RecordRange[api.Record], error) {
	directory, err := record.Directory().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load directory: %w", err)
	}
	r, err := api.MakeRange(directory, opts.Start, *opts.Count, func(v *url.URL) (api.Record, error) {
		if opts.Expand == nil || !*opts.Expand {
			return &api.UrlRecord{Value: v}, nil
		}

		r, err := s.queryAccount(ctx, batch, batch.Account(v), false)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("expand directory entry: %w", err)
		}
		return r, nil
	})
	return r, errors.UnknownError.Wrap(err)
}

func (s *Querier) queryPendingRange(ctx context.Context, batch *database.Batch, record *database.Account, opts *api.RangeOptions) (*api.RecordRange[api.Record], error) {
	pending, err := record.Pending().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load pending: %w", err)
	}
	r, err := api.MakeRange(pending, opts.Start, *opts.Count, func(v *url.TxID) (api.Record, error) {
		if opts.Expand == nil || !*opts.Expand {
			return &api.TxIDRecord{Value: v}, nil
		}

		r, err := s.queryTransaction(ctx, batch, v)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("expand pending entry: %w", err)
		}
		return r, nil
	})
	return r, errors.UnknownError.Wrap(err)
}

func (s *Querier) queryMinorBlock(ctx context.Context, batch *database.Batch, minorIndex uint64, entryRange *api.RangeOptions) (*api.MinorBlockRecord, error) {
	if entryRange == nil {
		entryRange = new(api.RangeOptions)
		entryRange.Count = new(uint64)
		*entryRange.Count = defaultPageSize
	}

	var ledger *protocol.BlockLedger
	err := batch.Account(s.partition.BlockLedger(minorIndex)).Main().GetAs(&ledger)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load block ledger: %w", err)
	}

	// allEntries, err := normalizeBlockEntries(batch, ledger)
	// if err != nil {
	// 	return nil, errors.InternalError.Wrap(err)
	// }
	allEntries := ledger.Entries

	// Convert entries into records
	r := new(api.MinorBlockRecord)
	r.Index = ledger.Index
	r.Time = &ledger.Time
	r.Source = s.partition.URL
	r.Entries = new(api.RecordRange[*api.ChainEntryRecord[api.Record]])
	r.Entries.Total = uint64(len(allEntries))

	var didAnchor *protocol.BlockEntry
	allocRange(r.Entries, entryRange, zeroBased)
	for i := range r.Entries.Records {
		e := allEntries[r.Entries.Start+uint64(i)]
		if s.partition.AnchorPool().Equal(e.Account) && e.Chain == "main" {
			didAnchor = e
		}

		r.Entries.Records[i], err = loadBlockEntry(batch, e)
		if err == nil {
			continue
		}

		if errors.Code(err).IsServerError() {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	if didAnchor == nil || !s.partition.Equal(protocol.DnUrl()) {
		return r, nil
	}

	r.Anchored, err = s.loadAnchoredBlocks(ctx, batch, didAnchor.Index)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return r, nil
}

func (s *Querier) queryMinorBlockRange(ctx context.Context, batch *database.Batch, minorRange *api.RangeOptions, omitEmpty bool) (*api.RecordRange[*api.MinorBlockRecord], error) {
	var ledger *protocol.SystemLedger
	err := batch.Account(s.partition.Ledger()).Main().GetAs(&ledger)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load system ledger: %w", err)
	}

	r := new(api.RecordRange[*api.MinorBlockRecord])
	r.Total = ledger.Index
	allocRange(r, minorRange, oneBased)

	r.Records, err = s.queryMinorBlockRange2(ctx, batch, r.Records, r.Start, ledger.Index, omitEmpty)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return r, nil
}

func (s *Querier) queryMajorBlock(ctx context.Context, batch *database.Batch, majorIndex uint64, minorRange *api.RangeOptions, omitEmpty bool) (*api.MajorBlockRecord, error) {
	if minorRange == nil {
		minorRange = new(api.RangeOptions)
		minorRange.Count = new(uint64)
		*minorRange.Count = defaultPageSize
	}

	chain, err := batch.Account(s.partition.AnchorPool()).MajorBlockChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load major block chain: %w", err)
	}
	if chain.Height() == 0 {
		return nil, errors.NotFound.WithFormat("no major blocks exist")
	}

	if majorIndex == 0 { // We don't have major block 0, avoid crash
		majorIndex = 1
	}

	entryIndex, entry, err := indexing.SearchIndexChain(chain, majorIndex-1, indexing.MatchExact, indexing.SearchIndexChainByBlock(majorIndex))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get major block %d: %w", majorIndex, err)
	}

	rootEntry, rootPrev, err := getMajorBlockBounds(s.partition, batch, entry, entryIndex)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	r := new(api.MajorBlockRecord)
	r.Index = majorIndex
	r.Time = *entry.BlockTime
	r.MinorBlocks = new(api.RecordRange[*api.MinorBlockRecord])
	r.MinorBlocks.Total = rootEntry.BlockIndex - rootPrev.BlockIndex
	allocRange(r.MinorBlocks, minorRange, zeroBased)

	r.MinorBlocks.Records, err = s.queryMinorBlockRange2(ctx, batch, r.MinorBlocks.Records, rootPrev.BlockIndex+1, rootEntry.BlockIndex, omitEmpty)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	return r, nil
}

func (s *Querier) queryMajorBlockRange(ctx context.Context, batch *database.Batch, major *api.RangeOptions, omitEmpty bool) (*api.RecordRange[*api.MajorBlockRecord], error) {
	chain, err := batch.Account(s.partition.AnchorPool()).MajorBlockChain().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load major block chain: %w", err)
	}
	if chain.Height() == 0 {
		r := new(api.RecordRange[*api.MajorBlockRecord])
		r.Records = []*api.MajorBlockRecord{}
		return r, nil
	}

	r := new(api.RecordRange[*api.MajorBlockRecord])
	{
		last := new(protocol.IndexEntry)
		err = chain.EntryAs(chain.Height()-1, last)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load last major block entry: %w", err)
		}
		r.Total = last.BlockIndex
	}
	allocRange(r, major, oneBased)

	if r.Start == 0 { // We don't have major block 0, avoid crash
		r.Start = 1
	}

	index, _, err := indexing.SearchIndexChain(chain, r.Start-1, indexing.MatchAfter, indexing.SearchIndexChainByBlock(r.Start))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get major block %d: %w", r.Start, err)
	}

	var i int
	nextBlock := r.Start
	for i < len(r.Records) && index < uint64(chain.Height()) {
		entry := new(protocol.IndexEntry)
		err = chain.EntryAs(int64(index), entry)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("get major block chain entry %d: %w", index, err)
		}

		if !omitEmpty {
			// Add nil entries if there are missing blocks
			for nextBlock < entry.BlockIndex {
				i++
				nextBlock++
			}
			if i >= len(r.Records) {
				break
			}
		}

		rootEntry, rootPrev, err := getMajorBlockBounds(s.partition, batch, entry, index)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}

		block := new(api.MajorBlockRecord)
		block.Index = entry.BlockIndex
		block.Time = *entry.BlockTime
		r.Records[i] = block

		// This query should only provide the _number_ of minor blocks in each
		// major block
		block.MinorBlocks = new(api.RecordRange[*api.MinorBlockRecord])
		block.MinorBlocks.Total = rootEntry.BlockIndex - rootPrev.BlockIndex

		index++
		i++
		nextBlock = entry.BlockIndex + 1
	}
	if i < len(r.Records) {
		r.Records = r.Records[:i]
	}
	return r, nil
}

func (s *Querier) queryMinorBlockRange2(ctx context.Context, batch *database.Batch, blocks []*api.MinorBlockRecord, blockIndex, maxIndex uint64, omitEmpty bool) ([]*api.MinorBlockRecord, error) {
	var i int
	var err error
	for i < len(blocks) && blockIndex <= maxIndex {
		blocks[i], err = s.queryMinorBlock(ctx, batch, blockIndex, nil)
		if err == nil {
			// Got a block
			i++
			blockIndex++
			continue
		}

		if errors.Code(err) == errors.NotFound {
			if !omitEmpty {
				blocks[i] = &api.MinorBlockRecord{Index: blockIndex}
				i++
			}
			blockIndex++
			continue
		}

		var e2 *errors.Error
		if !errors.As(err, &e2) || e2.Code.IsServerError() {
			return nil, errors.UnknownError.Wrap(err)
		}
	}
	if i < len(blocks) {
		blocks = blocks[:i]
	}
	return blocks, nil
}

func (s *Querier) searchForAnchor(ctx context.Context, batch *database.Batch, record *database.Account, hash []byte, wantReceipt bool) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	chains, err := record.Chains().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load chains index: %w", err)
	}

	rr := new(api.RecordRange[*api.ChainEntryRecord[api.Record]])
	for _, c := range chains {
		if c.Type != merkle.ChainTypeAnchor {
			continue
		}

		chain, err := record.ChainByName(c.Name)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("get chain %s: %w", c.Name, err)
		}

		r, err := s.queryChainEntryByValue(ctx, batch, chain, hash, true, wantReceipt)
		if err == nil {
			rr.Total++
			rr.Records = append(rr.Records, r)
		} else if !errors.Is(err, errors.NotFound) {
			return nil, errors.UnknownError.WithFormat("query chain entry: %w", err)
		}
	}
	if rr.Total == 0 {
		return nil, errors.NotFound.WithFormat("anchor %X not found", hash[:4])
	}
	return rr, nil
}

func (s *Querier) searchForKeyEntry(ctx context.Context, batch *database.Batch, scope *url.URL, search func(protocol.Signer) (int, protocol.KeyEntry, bool)) (*api.RecordRange[*api.KeyRecord], error) {
	auth, err := getAccountAuthoritySet(batch, scope)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load authority set: %w", err)
	}

	// For each local authority
	rs := new(api.RecordRange[*api.KeyRecord])
	for _, entry := range auth.Authorities {
		if !entry.Url.LocalTo(scope) {
			continue // Skip remote authorities
		}

		var authority protocol.Authority
		err = batch.Account(entry.Url).Main().GetAs(&authority)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load authority %v: %w", entry.Url, err)
		}

		// For each signer
		for _, signerUrl := range authority.GetSigners() {
			var signer protocol.Signer
			err = batch.Account(signerUrl).Main().GetAs(&signer)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("load signer %v: %w", signerUrl, err)
			}

			// Check for a matching entry
			i, e, ok := search(signer)
			if !ok {
				continue
			}

			// Found it!
			rec := new(api.KeyRecord)
			rec.Authority = entry.Url
			rec.Signer = signerUrl
			rec.Version = signer.GetVersion()
			rec.Index = uint64(i)

			if ks, ok := e.(*protocol.KeySpec); ok {
				rec.Entry = ks
			} else {
				rec.Entry = new(protocol.KeySpec)
				rec.Entry.LastUsedOn = e.GetLastUsedOn()
			}
			rs.Records = append(rs.Records, rec)
		}
	}
	rs.Total = uint64(len(rs.Records))
	return rs, nil
}

func (s *Querier) searchForTransactionHash(ctx context.Context, batch *database.Batch, hash [32]byte) (*api.RecordRange[*api.TxIDRecord], error) {
	var msg messaging.MessageWithTransaction
	err := batch.Message(hash).Main().GetAs(&msg)
	switch {
	case errors.Is(err, errors.NotFound):
		return new(api.RecordRange[*api.TxIDRecord]), nil

	case err != nil:
		return nil, errors.UnknownError.Wrap(err)

	default:
		// TODO Replace with principal or signer as appropriate
		txid := s.partition.WithTxID(msg.Hash())
		r := new(api.RecordRange[*api.TxIDRecord])
		r.Total = 1
		r.Records = []*api.TxIDRecord{{Value: txid}}
		return r, nil
	}
}

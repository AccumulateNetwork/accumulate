package api

import (
	"context"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

const defaultPageSize = 50
const maxPageSize = 100

type QueryService struct {
	logger    logging.OptionalLogger
	db        database.Beginner
	partition config.NetworkUrl
}

var _ api.QueryService = (*QueryService)(nil)

type QueryServiceParams struct {
	Logger    log.Logger
	Database  database.Beginner
	Partition string
}

func NewQueryService(params QueryServiceParams) *QueryService {
	s := new(QueryService)
	s.logger.L = params.Logger
	s.db = params.Database
	s.partition.URL = protocol.PartitionUrl(params.Partition)
	return s
}

func (s *QueryService) QueryRecord(ctx context.Context, account *url.URL, fragment []string, opts api.QueryRecordOptions) (api.Record, error) {
	batch := s.db.Begin(false)
	defer batch.Discard()

	if txid, err := account.AsTxID(); err == nil {
		h := txid.Hash()
		return s.queryTransaction(ctx, batch.Transaction(h[:]))
	}

	if len(fragment) == 0 {
		return s.queryAccount(ctx, batch, batch.Account(account), opts.IncludeReceipt)
	}

	switch strings.ToLower(fragment[0]) {
	case "anchor":
		if len(fragment) != 2 {
			return nil, errors.Format(errors.StatusBadRequest, "invalid fragment")
		}

		hash, err := decodeHash(fragment[1])
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "decode anchor: %w", err)
		}
		return s.queryAnchor(ctx, batch, batch.Account(account), hash, opts.IncludeReceipt)

	case "chain":
		switch len(fragment) {
		case 1:
			return s.queryChains(ctx, batch.Account(account))

		case 2:
			return s.queryChainByName(ctx, batch.Account(account), fragment[1])

		case 3:
			record, err := batch.Account(account).ChainByName(fragment[1])
			if err != nil {
				return nil, errors.Format(errors.StatusInternalError, "get chain %s: %w", fragment[1], err)
			}

			index, hash, err := decodeHashOrNumber(fragment[2])
			if err != nil {
				return nil, errors.Format(errors.StatusUnknownError, "decode entry: %w", err)
			}
			if hash == nil {
				return s.queryChainEntryByIndex(ctx, batch, record, index, true, opts.IncludeReceipt)
			}
			return s.queryChainEntryByValue(ctx, batch, record, hash, true, opts.IncludeReceipt)

		default:
			return nil, errors.Format(errors.StatusBadRequest, "invalid fragment")
		}

	case "data":
		if len(fragment) != 2 {
			return nil, errors.Format(errors.StatusBadRequest, "invalid fragment")
		}

		if strings.EqualFold(fragment[1], "latest") {
			return s.queryLatestDataEntry(ctx, batch, indexing.Data(batch, account))
		}

		index, hash, err := decodeHashOrNumber(fragment[2])
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "decode entry: %w", err)
		}
		if hash == nil {
			return s.queryDataEntryByIndex(ctx, batch, indexing.Data(batch, account), index)
		}
		return s.queryDataEntryByHash(ctx, batch, indexing.Data(batch, account), hash)
	}

	return nil, errors.Format(errors.StatusBadRequest, "invalid fragment")
}

func (s *QueryService) queryAccount(ctx context.Context, batch *database.Batch, record *database.Account, wantReceipt bool) (api.Record, error) {
	r := new(api.AccountRecord)

	state, err := record.Main().Get()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load state: %w", err)
	}
	r.Account = state

	switch state.Type() {
	case protocol.AccountTypeIdentity, protocol.AccountTypeKeyBook:
		directory, err := record.Directory().Get()
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "load directory: %w", err)
		}
		r.Directory, _ = api.MakeRange(directory, 0, defaultPageSize, func(v *url.URL) (*api.UrlRecord, error) {
			return &api.UrlRecord{Value: v}, nil
		})
		r.Directory.Total = uint64(len(directory))
	}

	pending, err := record.Pending().Get()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load pending: %w", err)
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
		return nil, errors.Format(errors.StatusUnknownError, "get state receipt: %w", err)
	}

	r.Receipt = new(api.Receipt)
	r.Receipt.Receipt = *receipt
	r.Receipt.LocalBlock = block
	return r, nil
}

func (s *QueryService) queryTransaction(ctx context.Context, record *database.Transaction) (*api.TransactionRecord, error) {
	state, err := record.Main().Get()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load state: %w", err)
	}
	if state.Transaction == nil {
		return nil, errors.Format(errors.StatusConflict, "record is not a transaction")
	}

	status, err := record.Status().Get()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load status: %w", err)
	}

	produced, err := record.Produced().Get()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load produced: %w", err)
	}

	r := new(api.TransactionRecord)
	r.TxID = state.Transaction.ID()
	r.Transaction = state.Transaction
	r.Status = status
	r.Produced = produced
	return r, nil
}

func (s *QueryService) querySignature(ctx context.Context, record *database.Transaction) (api.Record, error) {
	state, err := record.Main().Get()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load state: %w", err)
	}
	if state.Signature == nil {
		return nil, errors.Format(errors.StatusConflict, "record is not a signature")
	}

	r := new(api.SignatureRecord)
	r.Signature = state.Signature
	r.TxID = state.Txid
	return r, nil
}

func (s *QueryService) queryAnchor(ctx context.Context, batch *database.Batch, record *database.Account, hash []byte, wantReceipt bool) (api.Record, error) {
	chains, err := record.Chains().Get()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load chains index: %w", err)
	}

	for _, c := range chains {
		if c.Type != managed.ChainTypeAnchor {
			continue
		}

		chain, err := record.ChainByName(c.Name)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "get chain %s: %w", c.Name, err)
		}

		r, err := s.queryChainEntryByValue(ctx, batch, chain, hash, true, wantReceipt)
		if err == nil {
			return r, nil
		}
		if !errors.Is(err, errors.StatusNotFound) {
			return nil, errors.Format(errors.StatusUnknownError, "query chain entry: %w", err)
		}
	}
	return nil, errors.NotFound("anchor %X not found", hash[:4])
}

func (s *QueryService) queryChains(ctx context.Context, record *database.Account) (api.Record, error) {
	chains, err := record.Chains().Get()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load chains index: %w", err)
	}

	r := new(api.RecordRange[*api.ChainRecord])
	r.Total = uint64(len(chains))
	r.Records = make([]*api.ChainRecord, len(chains))
	for i, c := range chains {
		cr, err := s.queryChainByName(ctx, record, c.Name)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "chain %s: %w", c.Name, err)
		}

		r.Records[i] = cr
	}
	return r, nil
}

func (s *QueryService) queryChainByName(ctx context.Context, record *database.Account, name string) (*api.ChainRecord, error) {
	chain, err := record.ChainByName(name)
	if err != nil {
		return nil, errors.Format(errors.StatusInternalError, "get chain %s: %w", name, err)
	}

	return s.queryChain(ctx, chain)
}

func (s *QueryService) queryChain(ctx context.Context, record *database.Chain2) (*api.ChainRecord, error) {
	head, err := record.Head().Get()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load head: %w", err)
	}

	r := new(api.ChainRecord)
	r.Name = record.Name()
	r.Type = record.Type()
	r.Count = uint64(head.Count)
	r.State = head.Pending
	return r, nil
}

func (s *QueryService) queryChainEntryByIndex(ctx context.Context, batch *database.Batch, record *database.Chain2, index uint64, expand, wantReceipt bool) (api.Record, error) {
	value, err := record.Entry(int64(index))
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "get entry: %w", err)
	}
	return s.queryChainEntry(ctx, batch, record, uint64(index), value, expand, wantReceipt)
}

func (s *QueryService) queryChainEntryByValue(ctx context.Context, batch *database.Batch, record *database.Chain2, value []byte, expand, wantReceipt bool) (api.Record, error) {
	index, err := record.IndexOf(value)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "get entry index: %w", err)
	}
	return s.queryChainEntry(ctx, batch, record, uint64(index), value, expand, wantReceipt)
}

func (s *QueryService) queryChainEntry(ctx context.Context, batch *database.Batch, record *database.Chain2, index uint64, value []byte, expand, wantReceipt bool) (api.Record, error) {
	r := new(api.ChainEntryRecord[api.Record])
	r.Name = record.Name()
	r.Type = record.Type()
	r.Index = index
	r.Entry = *(*[32]byte)(value)

	if expand {
		switch r.Type {
		case managed.ChainTypeIndex:
			v := new(protocol.IndexEntry)
			if v.UnmarshalBinary(value) == nil {
				r.Value = &api.IndexEntryRecord{Value: v}
			}

		case managed.ChainTypeTransaction:
			record := batch.Transaction(value)
			var typ string
			var err error
			if strings.EqualFold(r.Name, "signature") {
				typ = "signature"
				r.Value, err = s.querySignature(ctx, record)
			} else {
				typ = "transaction"
				r.Value, err = s.queryTransaction(ctx, record)
			}
			if err != nil {
				return nil, errors.Format(errors.StatusUnknownError, "load %s: %w", typ, err)
			}
		}
	}

	if wantReceipt {
		block, rootIndexIndex, receipt, err := indexing.ReceiptForChainIndex(s.partition, batch, record, int64(index))
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "get chain receipt: %w", err)
		}

		r.Receipt = new(api.Receipt)
		r.Receipt.Receipt = *receipt
		r.Receipt.LocalBlock = block

		// Find the major block
		rxc, err := batch.Account(s.partition.AnchorPool()).MajorBlockChain().Get()
		if err != nil {
			return nil, errors.Format(errors.StatusInternalError, "load root index chain: %w", err)
		}
		_, major, err := indexing.SearchIndexChain(rxc, 0, indexing.MatchAfter, indexing.SearchIndexChainByRootIndexIndex(rootIndexIndex))
		switch {
		case err == nil:
			r.Receipt.MajorBlock = major.BlockIndex
		case errors.Is(err, errors.StatusNotFound):
			// Not in a major block yet
		default:
			return nil, errors.Format(errors.StatusInternalError, "locate major block for root index entry %d: %w", rootIndexIndex, err)
		}
	}

	return r, nil
}

func (s *QueryService) queryLatestDataEntry(ctx context.Context, batch *database.Batch, record *indexing.DataIndexer) (api.Record, error) {
	count, err := record.Count()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "get count: %w", err)
	}

	if count == 0 {
		return nil, errors.NotFound("empty")
	}

	return s.queryDataEntryByIndex(ctx, batch, record, count-1)
}

func (s *QueryService) queryDataEntryByIndex(ctx context.Context, batch *database.Batch, record *indexing.DataIndexer, index uint64) (api.Record, error) {
	entryHash, err := record.Entry(index)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "get entry hash: %w", err)
	}

	return s.queryDataEntry(ctx, batch, record, index, entryHash)
}

func (s *QueryService) queryDataEntryByHash(ctx context.Context, batch *database.Batch, record *indexing.DataIndexer, entryHash []byte) (api.Record, error) {
	// TODO: Find a way to get the index without scanning the entire set
	return s.queryDataEntry(ctx, batch, record, 0, entryHash)
}

func (s *QueryService) queryDataEntry(ctx context.Context, batch *database.Batch, record *indexing.DataIndexer, index uint64, entryHash []byte) (api.Record, error) {
	txnHash, err := record.Transaction(entryHash)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "get transaction hash: %w", err)
	}

	r := new(api.ChainEntryRecord[*api.TransactionRecord])
	r.Name = "data"
	r.Type = protocol.ChainTypeTransaction
	r.Index = index
	r.Entry = *(*[32]byte)(entryHash)

	r.Value, err = s.queryTransaction(ctx, batch.Transaction(txnHash))
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load transaction: %w", err)
	}

	return r, nil
}

func (s *QueryService) QueryRange(ctx context.Context, account *url.URL, fragment []string, opts api.QueryRangeOptions) (*api.RecordRange[api.Record], error) {
	if len(fragment) == 0 {
		return nil, errors.NotFound("fragment not found")
	}

	batch := s.db.Begin(false)
	defer batch.Discard()

	if opts.Count == nil {
		opts.Count = new(uint64)
		*opts.Count = defaultPageSize
	} else if *opts.Count > maxPageSize {
		*opts.Count = maxPageSize
	}

	switch fragment[0] {
	case "chain":
		if len(fragment) != 2 {
			return nil, errors.NotFound("fragment not found")
		}
		chain, err := batch.Account(account).ChainByName(fragment[1])
		if err != nil {
			return nil, errors.Format(errors.StatusInternalError, "get chain %s: %w", fragment[1], err)
		}
		return s.queryChainEntryRange(ctx, batch, chain, opts)

	case "data":
		if len(fragment) != 1 {
			return nil, errors.NotFound("fragment not found")
		}
		return s.queryDataEntryRange(ctx, batch, indexing.Data(batch, account), opts)

	case "directory":
		if len(fragment) != 1 {
			return nil, errors.NotFound("fragment not found")
		}
		return s.queryDirectoryRange(ctx, batch, batch.Account(account), opts)

	case "pending":
		if len(fragment) != 1 {
			return nil, errors.NotFound("fragment not found")
		}
		return s.queryPendingRange(ctx, batch, batch.Account(account), opts)

	default:
		return nil, errors.NotFound("fragment not found")
	}
}

func (s *QueryService) queryChainEntryRange(ctx context.Context, batch *database.Batch, record *database.Chain2, opts api.QueryRangeOptions) (*api.RecordRange[api.Record], error) {
	head, err := record.Head().Get()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "get head: %w", err)
	}

	r := new(api.RecordRange[api.Record])
	r.Start = opts.Start
	r.Total = uint64(head.Count)

	if r.Start >= r.Total {
		return r, nil
	}

	if r.Start+*opts.Count >= r.Total {
		*opts.Count = r.Total - r.Start
	}

	r.Records = make([]api.Record, *opts.Count)
	for i := range r.Records {
		r.Records[i], err = s.queryChainEntryByIndex(ctx, batch, record, r.Start+uint64(i), opts.Expand, false)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "get entry %d: %w", r.Start+uint64(i), err)
		}
	}
	return r, nil
}

func (s *QueryService) queryDataEntryRange(ctx context.Context, batch *database.Batch, record *indexing.DataIndexer, opts api.QueryRangeOptions) (*api.RecordRange[api.Record], error) {
	r := new(api.RecordRange[api.Record])
	r.Start = opts.Start

	var err error
	r.Total, err = record.Count()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "get count: %w", err)
	}

	if r.Start >= r.Total {
		return r, nil
	}

	if r.Start+*opts.Count >= r.Total {
		*opts.Count = r.Total - r.Start
	}

	r.Records = make([]api.Record, *opts.Count)
	for i := range r.Records {
		r.Records[i], err = s.queryDataEntryByIndex(ctx, batch, record, r.Start+uint64(i))
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "get entry %d: %w", r.Start+uint64(i), err)
		}
	}
	return r, nil
}

func (s *QueryService) queryDirectoryRange(ctx context.Context, batch *database.Batch, record *database.Account, opts api.QueryRangeOptions) (*api.RecordRange[api.Record], error) {
	directory, err := record.Directory().Get()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load directory: %w", err)
	}
	r, err := api.MakeRange(directory, opts.Start, *opts.Count, func(v *url.URL) (api.Record, error) {
		if !opts.Expand {
			return &api.UrlRecord{Value: v}, nil
		}

		r, err := s.queryAccount(ctx, batch, batch.Account(v), false)
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "expand directory entry: %w", err)
		}
		return r, nil
	})
	return r, errors.Wrap(errors.StatusUnknownError, err)
}

func (s *QueryService) queryPendingRange(ctx context.Context, batch *database.Batch, record *database.Account, opts api.QueryRangeOptions) (*api.RecordRange[api.Record], error) {
	pending, err := record.Pending().Get()
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load pending: %w", err)
	}
	r, err := api.MakeRange(pending, opts.Start, *opts.Count, func(v *url.TxID) (api.Record, error) {
		if !opts.Expand {
			return &api.TxIDRecord{Value: v}, nil
		}

		h := v.Hash()
		r, err := s.queryTransaction(ctx, batch.Transaction(h[:]))
		if err != nil {
			return nil, errors.Format(errors.StatusUnknownError, "expand pending entry: %w", err)
		}
		return r, nil
	})
	return r, errors.Wrap(errors.StatusUnknownError, err)
}

func decodeHash(s string) ([]byte, error) {
	h, err := hex.DecodeString(s)
	if err != nil {
		return nil, errors.Format(errors.StatusBadRequest, "not a hash: %w", err)
	}
	if len(h) != 32 {
		return nil, errors.Format(errors.StatusBadRequest, "not a hash: wrong length: want 32, got %d", len(h))
	}
	return h, nil
}

func decodeHashOrNumber(s string) (uint64, []byte, error) {
	if i, err := strconv.ParseUint(s, 10, 64); err == nil {
		return i, nil, nil
	}

	if h, err := hex.DecodeString(s); err != nil {
		if len(h) != 32 {
			return 0, nil, errors.Format(errors.StatusBadRequest, "not a hash: wrong length: want 32, got %d", len(h))
		}
		return 0, h, nil
	}

	return 0, nil, errors.New(errors.StatusBadRequest, "neither a hash nor a number")
}

package chain

import (
	"encoding"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/query"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

func (m *Executor) queryByUrl(batch *database.Batch, u *url.URL) ([]byte, encoding.BinaryMarshaler, error) {
	qv := u.QueryValues()
	switch {
	case qv.Get("txid") != "":
		// Query by transaction ID
		txid, err := hex.DecodeString(qv.Get("txid"))
		if err != nil {
			return nil, nil, fmt.Errorf("invalid txid %q: %v", qv.Get("txid"), err)
		}

		v, err := m.queryByTxId(batch, txid)
		return []byte("tx"), v, err

	default:
		// Query by chain URL
		v, err := m.queryByChainId(batch, u.ResourceChain())
		return []byte("chain"), v, err
	}
}

func (m *Executor) queryByChainId(batch *database.Batch, chainId []byte) (*query.ResponseByChainId, error) {
	qr := query.ResponseByChainId{}

	// Look up record or transaction
	obj, err := batch.RecordByID(chainId).GetState()
	if errors.Is(err, storage.ErrNotFound) {
		obj, err = batch.Transaction(chainId).GetState()
	}
	// Not a record or a transaction
	if errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("%w: no chain or transaction found for %X", storage.ErrNotFound, chainId)
	}
	// Some other error
	if err != nil {
		return nil, fmt.Errorf("failed to locate chain entry for chain id %x: %v", chainId, err)
	}

	qr.Object.Entry, err = obj.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %T (id %x): %v", obj, chainId, err)
	}

	// Add Merkle chain info (for records)
	chain, err := batch.RecordByID(chainId).Chain(protocol.MainChain)
	if err == nil {
		qr.Height = uint64(chain.Height())

		pending := chain.Pending()
		qr.Roots = make([][]byte, len(pending))
		for i, h := range pending {
			qr.Roots[i] = h
		}
	}

	return &qr, nil
}

func (m *Executor) queryDirectoryByChainId(batch *database.Batch, chainId []byte, start uint64, limit uint64) (*protocol.DirectoryQueryResult, error) {
	md, err := m.loadDirectoryMetadata(batch, chainId)
	if err != nil {
		return nil, err
	}

	count := limit
	if start+count > md.Count {
		count = md.Count - start
	}
	if count > md.Count { // when uint64 0-x is really big number
		count = 0
	}

	resp := new(protocol.DirectoryQueryResult)
	resp.Entries = make([]string, count)

	for i := uint64(0); i < count; i++ {
		resp.Entries[i], err = m.loadDirectoryEntry(batch, chainId, start+i)
		if err != nil {
			return nil, fmt.Errorf("failed to get entry %d", i)
		}
	}
	resp.Total = md.Count

	return resp, nil
}

func (m *Executor) queryByTxId(batch *database.Batch, txid []byte) (*query.ResponseByTxId, error) {
	var err error

	tx := batch.Transaction(txid)
	txState, err := tx.GetState()
	if errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("tx %X %w", txid, storage.ErrNotFound)
	} else if err != nil {
		return nil, fmt.Errorf("invalid query from GetTx in state database, %v", err)
	}

	pending := new(state.PendingTransaction)
	pending.Type = types.ChainTypePendingTransaction
	pending.ChainUrl = txState.ChainUrl
	pending.TransactionState = &txState.TxState

	status, err := tx.GetStatus()
	if err != nil {
		return nil, fmt.Errorf("invalid query from GetTx in state database, %v", err)
	} else if !status.Delivered && status.Remote {
		// If the transaction is a synthetic transaction produced by this BVN
		// and has not been delivered, pretend like it doesn't exist
		return nil, fmt.Errorf("tx %X %w", txid, storage.ErrNotFound)
	}

	pending.Status, err = status.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("invalid query from GetTx in state database, %v", err)
	}

	pending.Signature, err = tx.GetSignatures()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("invalid query from GetTx in state database, %v", err)
	}

	qr := query.ResponseByTxId{}
	txObj := new(state.Object)
	txObj.Entry, err = txState.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tx state: %v", err)
	}
	qr.TxState, err = txObj.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tx state: %v", err)
	}

	txObj.Entry, err = pending.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tx state: %v", err)
	}
	qr.TxPendingState, err = txObj.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tx state: %v", err)
	}

	synth, err := tx.GetSyntheticTxns()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("invalid query from GetTx in state database, %v", err)
	}

	qr.TxSynthTxIds = make(types.Bytes, 0, len(synth)*32)
	for _, synth := range synth {
		qr.TxSynthTxIds = append(qr.TxSynthTxIds, synth[:]...)
	}

	err = qr.TxId.FromBytes(txid)
	if err != nil {
		return nil, fmt.Errorf("txid in query cannot be converted, %v", err)
	}

	return &qr, nil
}

func (m *Executor) queryDataByUrl(batch *database.Batch, u *url.URL) (*protocol.ResponseDataEntry, error) {
	qr := protocol.ResponseDataEntry{}

	data, err := batch.Record(u).Data()
	if err != nil {
		return nil, err
	}

	entryHash, entry, err := data.GetLatest()
	if err != nil {
		return nil, err
	}

	copy(qr.EntryHash[:], entryHash)
	qr.Entry = *entry
	return &qr, nil
}

func (m *Executor) queryDataByEntryHash(batch *database.Batch, u *url.URL, entryHash []byte) (*protocol.ResponseDataEntry, error) {
	qr := protocol.ResponseDataEntry{}
	copy(qr.EntryHash[:], entryHash)

	data, err := batch.Record(u).Data()
	if err != nil {
		return nil, err
	}

	entry, err := data.Get(entryHash)
	if err != nil {
		return nil, err
	}

	qr.Entry = *entry
	return &qr, nil
}

func (m *Executor) queryDataSet(batch *database.Batch, u *url.URL, start int64, limit int64, expand bool) (*protocol.ResponseDataEntrySet, error) {
	qr := protocol.ResponseDataEntrySet{}

	data, err := batch.Record(u).Data()
	if err != nil {
		return nil, err
	}

	entryHashes, err := data.GetHashes(start, start+limit)
	if err != nil {
		return nil, err
	}

	qr.Total = uint64(data.Height())
	for _, entryHash := range entryHashes {
		er := protocol.ResponseDataEntry{}
		copy(er.EntryHash[:], entryHash)

		if expand {
			entry, err := data.Get(entryHash)
			if err != nil {
				return nil, err
			}
			er.Entry = *entry
		}

		qr.DataEntries = append(qr.DataEntries, er)
	}
	return &qr, nil
}

func (m *Executor) Query(q *query.Query) (k, v []byte, err *protocol.Error) {
	batch := m.DB.Begin()
	defer batch.Discard()

	switch q.Type {
	case types.QueryTypeTxId:
		txr := query.RequestByTxId{}
		err := txr.UnmarshalBinary(q.Content)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeUnMarshallingError, Message: err}
		}
		qr, err := m.queryByTxId(batch, txr.TxId[:])
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeTxnQueryError, Message: err}
		}
		k = []byte("tx")
		v, err = qr.MarshalBinary()
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeMarshallingError, Message: fmt.Errorf("%v, on Chain %x", err, txr.TxId[:])}
		}
	case types.QueryTypeTxHistory:
		txh := query.RequestTxHistory{}
		err := txh.UnmarshalBinary(q.Content)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeUnMarshallingError, Message: err}
		}

		chain, err := batch.RecordByID(txh.ChainId[:]).Chain(protocol.MainChain)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeTxnHistory, Message: fmt.Errorf("error obtaining txid range %v", err)}
		}

		thr := query.ResponseTxHistory{}
		thr.Total = chain.Height()

		txids, err := chain.Entries(txh.Start, txh.Start+txh.Limit)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeTxnHistory, Message: fmt.Errorf("error obtaining txid range %v", err)}
		}

		for _, txid := range txids {
			qr, err := m.queryByTxId(batch, txid)
			if err != nil {
				return nil, nil, &protocol.Error{Code: protocol.CodeTxnQueryError, Message: err}
			}
			thr.Transactions = append(thr.Transactions, *qr)
		}
		k = []byte("tx-history")
		v, err = thr.MarshalBinary()
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeMarshallingError, Message: fmt.Errorf("error marshalling payload for transaction history")}
		}
	case types.QueryTypeUrl:
		chr := query.RequestByUrl{}
		err := chr.UnmarshalBinary(q.Content)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeUnMarshallingError, Message: err}
		}
		u, err := url.Parse(*chr.Url.AsString())
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeInvalidURL, Message: fmt.Errorf("invalid URL in query %s", chr.Url)}
		}

		var obj encoding.BinaryMarshaler
		k, obj, err = m.queryByUrl(batch, u)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeTxnQueryError, Message: err}
		}
		v, err = obj.MarshalBinary()
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeMarshallingError, Message: fmt.Errorf("%v, on Url %s", err, chr.Url)}
		}
	case types.QueryTypeDirectoryUrl:
		chr := query.RequestDirectory{}
		err := chr.UnmarshalBinary(q.Content)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeUnMarshallingError, Message: err}
		}
		u, err := url.Parse(*chr.Url.AsString())
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeInvalidURL, Message: fmt.Errorf("invalid URL in query %s", chr.Url)}
		}
		dir, err := m.queryDirectoryByChainId(batch, u.ResourceChain(), chr.Start, chr.Limit)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeDirectoryURL, Message: err}
		}

		if chr.ExpandChains {
			entries, err := m.expandChainEntries(batch, dir.Entries)
			if err != nil {
				return nil, nil, &protocol.Error{Code: protocol.CodeDirectoryURL, Message: err}
			}
			dir.ExpandedEntries = entries
		}

		k = []byte("directory")
		v, err = dir.MarshalBinary()
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeMarshallingError, Message: fmt.Errorf("%v, on Url %s", err, chr.Url)}
		}
	case types.QueryTypeChainId:
		chr := query.RequestByChainId{}
		err := chr.UnmarshalBinary(q.Content)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeUnMarshallingError, Message: err}
		}
		obj, err := m.queryByChainId(batch, chr.ChainId[:])
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeChainIdError, Message: err}
		}
		k = []byte("chain")
		v, err = obj.MarshalBinary()
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeMarshallingError, Message: fmt.Errorf("%v, on Chain %x", err, chr.ChainId)}
		}
	case types.QueryTypeData:
		chr := query.RequestDataEntry{}
		err := chr.UnmarshalBinary(q.Content)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeUnMarshallingError, Message: err}
		}

		u, err := url.Parse(chr.Url)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeInvalidURL, Message: fmt.Errorf("invalid URL in query %s", chr.Url)}
		}

		var ret *protocol.ResponseDataEntry
		if chr.EntryHash != [32]byte{} {
			ret, err = m.queryDataByEntryHash(batch, u, chr.EntryHash[:])
			if err != nil {
				return nil, nil, &protocol.Error{Code: protocol.CodeDataEntryHashError, Message: err}
			}
		} else {
			ret, err = m.queryDataByUrl(batch, u)
			if err != nil {
				return nil, nil, &protocol.Error{Code: protocol.CodeDataUrlError, Message: err}
			}
		}

		k = []byte("data")
		v, err = ret.MarshalBinary()
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeMarshallingError, Message: err}
		}
	case types.QueryTypeDataSet:
		chr := query.RequestDataEntrySet{}
		err := chr.UnmarshalBinary(q.Content)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeUnMarshallingError, Message: err}
		}
		u, err := url.Parse(chr.Url)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeInvalidURL, Message: fmt.Errorf("invalid URL in query %s", chr.Url)}
		}

		ret, err := m.queryDataSet(batch, u, int64(chr.Start), int64(chr.Count), chr.ExpandChains)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeDataEntryHashError, Message: err}
		}

		k = []byte("dataSet")
		v, err = ret.MarshalBinary()
	case types.QueryTypeKeyPageIndex:
		chr := query.RequestKeyPageIndex{}
		err := chr.UnmarshalBinary(q.Content)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeUnMarshallingError, Message: err}
		}
		u, err := url.Parse(chr.Url)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeInvalidURL, Message: fmt.Errorf("invalid URL in query %s", chr.Url)}
		}
		obj, err := m.queryByChainId(batch, u.ResourceChain())
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeChainIdError, Message: err}
		}
		chainHeader := new(state.ChainHeader)
		if err = obj.As(chainHeader); err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeMarshallingError, Message: fmt.Errorf("inavid object error")}
		}
		if chainHeader.Type != types.ChainTypeKeyBook {
			u, err := url.Parse(string(chainHeader.KeyBook))
			if err != nil {
				return nil, nil, &protocol.Error{Code: protocol.CodeInvalidURL, Message: fmt.Errorf("invalid URL in query %s", chr.Url)}
			}
			obj, err = m.queryByChainId(batch, u.ResourceChain())
			if err != nil {
				return nil, nil, &protocol.Error{Code: protocol.CodeChainIdError, Message: err}
			}
			if err := obj.As(chainHeader); err != nil {
				return nil, nil, &protocol.Error{Code: protocol.CodeMarshallingError, Message: fmt.Errorf("inavid object error")}
			}
		}
		k = []byte("key-page-index")
		var found bool
		keyBook := new(protocol.KeyBook)
		if err = obj.As(keyBook); err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeMarshallingError, Message: fmt.Errorf("invalid object error")}
		}
		response := query.ResponseKeyPageIndex{
			KeyBook: keyBook.GetChainUrl(),
		}
		for index, page := range keyBook.Pages {
			u, err := url.Parse(page)
			if err != nil {
				return nil, nil, &protocol.Error{Code: protocol.CodeInvalidURL, Message: fmt.Errorf("invalid URL in query %s", u.String())}
			}
			pageObject, err := m.queryByChainId(batch, u.ResourceChain())
			if err != nil {
				return nil, nil, &protocol.Error{Code: protocol.CodeChainIdError, Message: err}
			}
			keyPage := new(protocol.KeyPage)
			if err = pageObject.As(keyPage); err != nil {
				return nil, nil, &protocol.Error{Code: protocol.CodeMarshallingError, Message: fmt.Errorf("invalid object error")}
			}
			if keyPage.FindKey([]byte(chr.Key)) != nil {
				response.KeyPage = keyPage.GetChainUrl()
				response.Index = uint64(index)
				found = true
				break
			}
		}
		if !found {
			return nil, nil, &protocol.Error{Code: protocol.CodeNotFound, Message: fmt.Errorf("key %s not found in keypage url %s", chr.Key, chr.Url)}
		}
		v, err = response.MarshalBinary()
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeMarshallingError, Message: err}
		}
	default:
		return nil, nil, &protocol.Error{Code: protocol.CodeInvalidQueryType, Message: fmt.Errorf("unable to query for type, %s (%d)", q.Type.Name(), q.Type.AsUint64())}
	}
	return k, v, err
}

func (m *Executor) expandChainEntries(batch *database.Batch, entries []string) ([]*state.Object, error) {
	expEntries := make([]*state.Object, len(entries))
	for i, entry := range entries {
		index := i
		r, err := m.expandChainEntry(batch, entry)
		if err != nil {
			return nil, err
		}
		expEntries[index] = r
	}
	return expEntries, nil
}

func (m *Executor) expandChainEntry(batch *database.Batch, entryUrl string) (*state.Object, error) {
	u, err := url.Parse(entryUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid URL in query %s", entryUrl)
	}

	v, err := m.queryByChainId(batch, u.ResourceChain())
	if err != nil {
		return nil, &protocol.Error{Code: protocol.CodeTxnQueryError, Message: err}
	}
	return &state.Object{Entry: v.Entry, Height: v.Height, Roots: v.Roots}, nil
}

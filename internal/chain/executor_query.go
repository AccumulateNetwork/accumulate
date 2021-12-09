package chain

import (
	"encoding"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/api/v2"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/query"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

func (m *Executor) queryByUrl(u *url.URL) ([]byte, encoding.BinaryMarshaler, error) {
	qv := u.QueryValues()
	switch {
	case qv.Get("txid") != "":
		// Query by transaction ID
		txid, err := hex.DecodeString(qv.Get("txid"))
		if err != nil {
			return nil, nil, fmt.Errorf("invalid txid %q: %v", qv.Get("txid"), err)
		}

		v, err := m.queryByTxId(txid)
		return []byte("tx"), v, err

	default:
		// Query by chain URL
		v, err := m.queryByChainId(u.ResourceChain())
		return []byte("chain"), v, err
	}
}

func (m *Executor) queryByChainId(chainId []byte) (*query.ResponseByChainId, error) {
	qr := query.ResponseByChainId{}

	// This intentionally uses GetPersistentEntry instead of GetCurrentEntry.
	// Callers should never see uncommitted values.
	obj, err := m.DB.GetPersistentEntry(chainId, false)
	// Or a transaction
	if errors.Is(err, storage.ErrNotFound) {
		obj, err = m.DB.GetTransaction(chainId)
	}
	// Not a state entry or a transaction
	if errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("%w: no chain or transaction found for %X", storage.ErrNotFound, chainId)
	}
	// Some other error
	if err != nil {
		return nil, fmt.Errorf("failed to locate chain entry for chain id %x: %v", chainId, err)
	}

	err = obj.As(new(state.ChainHeader))
	if err != nil {
		return nil, fmt.Errorf("unable to extract chain header for chain id %x: %v", chainId, err)
	}

	qr.Object = *obj
	return &qr, nil
}

func (m *Executor) queryDirectoryByChainId(chainId []byte, start uint64, limit uint64) (*protocol.DirectoryQueryResult, error) {
	md, err := m.loadDirectoryMetadata(chainId)
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
		resp.Entries[i], err = m.loadDirectoryEntry(chainId, start+i)
		if err != nil {
			return nil, fmt.Errorf("failed to get entry %d", i)
		}
	}
	resp.Total = md.Count

	return resp, nil
}

func (m *Executor) queryByTxId(txid []byte) (*query.ResponseByTxId, error) {
	var err error

	qr := query.ResponseByTxId{}
	qr.TxState, err = m.DB.GetTx(txid)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("tx %X %w", txid, storage.ErrNotFound)
	} else if err != nil {
		return nil, fmt.Errorf("invalid query from GetTx in state database, %v", err)
	}
	qr.TxPendingState, err = m.DB.GetPendingTx(txid)
	if !errors.Is(err, storage.ErrNotFound) && err != nil {
		//this is only an error if the pending states have not yet been purged or some other database error occurred
		return nil, fmt.Errorf("%w: error in query for pending chain on txid %X", storage.ErrNotFound, txid)
	}

	qr.TxSynthTxIds, err = m.DB.GetSyntheticTxIds(txid)
	if !errors.Is(err, storage.ErrNotFound) && err != nil {
		//this is only an error if the transactions produced synth tx's or some other database error occurred
		return nil, fmt.Errorf("%w: error in query for synthetic txid txid %X", storage.ErrNotFound, txid)
	}

	err = qr.TxId.FromBytes(txid)
	if err != nil {
		return nil, fmt.Errorf("txid in query cannot be converted, %v", err)
	}

	return &qr, nil
}

func (m *Executor) queryDataByUrl(u *url.URL) (*protocol.ResponseDataEntry, error) {
	qr := protocol.ResponseDataEntry{}

	entryHash, entry, err := m.DB.GetChainData(u.ResourceChain())
	if err != nil {
		return nil, err
	}

	copy(qr.EntryHash[:], entryHash)
	err = qr.Entry.UnmarshalBinary(entry)
	if err != nil {
		return nil, err
	}
	return &qr, nil
}

func (m *Executor) queryDataByEntryHash(u *url.URL, entryHash []byte) (*protocol.ResponseDataEntry, error) {
	qr := protocol.ResponseDataEntry{}

	entryHash, entry, err := m.DB.GetChainDataByEntryHash(u.ResourceChain(), entryHash)
	if err != nil {
		return nil, err
	}

	copy(qr.EntryHash[:], entryHash)
	err = qr.Entry.UnmarshalBinary(entry)
	if err != nil {
		return nil, err
	}
	return &qr, nil
}

func (m *Executor) queryDataSet(u *url.URL, start int64, limit int64, expand bool) (*protocol.ResponseDataEntrySet, error) {
	qr := protocol.ResponseDataEntrySet{}

	entryHashes, height, err := m.DB.GetChainDataRange(u.ResourceChain(), start, start+limit)
	if err != nil {
		return nil, err
	}

	qr.Total = uint64(height)
	for i := range entryHashes {
		if expand {
			entry, err := m.queryDataByEntryHash(u, entryHashes[i][:])
			if err != nil {
				return nil, err
			}
			qr.DataEntries = append(qr.DataEntries, *entry)
		} else {
			qr.DataEntries = append(qr.DataEntries, protocol.ResponseDataEntry{EntryHash: entryHashes[i]})
		}
	}
	return &qr, nil
}

func (m *Executor) Query(q *query.Query) (k, v []byte, err *protocol.Error) {
	switch q.Type {
	case types.QueryTypeTxId:
		txr := query.RequestByTxId{}
		err := txr.UnmarshalBinary(q.Content)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeUnMarshallingError, Message: err}
		}
		qr, err := m.queryByTxId(txr.TxId[:])
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

		thr := query.ResponseTxHistory{}
		txids, maxAmt, err := m.DB.GetChainRange(txh.ChainId[:], txh.Start, txh.Start+txh.Limit)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeTxnHistory, Message: fmt.Errorf("error obtaining txid range %v", err)}
		}
		thr.Total = maxAmt
		for i := range txids {
			qr, err := m.queryByTxId(txids[i][:])
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
		k, obj, err = m.queryByUrl(u)
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
		dir, err := m.queryDirectoryByChainId(u.ResourceChain(), chr.Start, chr.Limit)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeDirectoryURL, Message: err}
		}

		if chr.ExpandChains {
			entries, err := m.expandChainEntries(dir.Entries)
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
		obj, err := m.queryByChainId(chr.ChainId[:])
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
			ret, err = m.queryDataByEntryHash(u, chr.EntryHash[:])
			if err != nil {
				return nil, nil, &protocol.Error{Code: protocol.CodeDataEntryHashError, Message: err}
			}
		} else {
			ret, err = m.queryDataByUrl(u)
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

		ret, err := m.queryDataSet(u, int64(chr.Start), int64(chr.Count), chr.ExpandChains)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeDataEntryHashError, Message: err}
		}

		k = []byte("dataSet")
		v, err = ret.MarshalBinary()
	}
	case types.QueryTypeKeyPageIndices:
		chr := query.RequestByUrlAndKey{}
		u, err := url.Parse(*chr.Url.AsString())
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeInvalidURL, Message: fmt.Errorf("invalid URL in query %s", chr.Url)}
		}
		obj, err := m.queryByChainId(u.ResourceChain())
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeChainIdError, Message: err}
		}
		chainHeader := new(state.ChainHeader)
		if err = obj.As(chainHeader); err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeMarshallingError, Message: fmt.Errorf("inavid object error")}
		}
		if chainHeader.Type != types.ChainTypeKeyBook {
			obj, err = m.queryByChainId(chainHeader.KeyBook.Bytes())
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
		response := api.ResponseKeyPageIndex{
			KeyBook: keyBook.GetChainUrl(),
		}
		for index, page := range keyBook.Pages {
			pageObject, err := m.queryByChainId(page[:])
			if err != nil {
				return nil, nil, &protocol.Error{Code: protocol.CodeChainIdError, Message: err}
			}
			keyPage := new(protocol.KeyPage)
			if err = pageObject.As(keyPage); err != nil {
				return nil, nil, &protocol.Error{Code: protocol.CodeMarshallingError, Message: fmt.Errorf("invalid object error")}
			}
			if keyPage.FindKey([]byte(chr.Key)) != nil {
				response.KeyPage = keyPage.GetChainUrl()
				response.Index = index
				found = true
				break
			}
		}
		if !found {
			return nil, nil, &protocol.Error{Code: protocol.CodeNotFound, Message: fmt.Errorf("key %s not found in keypage url %s", chr.Key, chr.Url)}
		}
		v, err = json.Marshal(response)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeMarshallingError, Message: err}
		}
	default:
		return nil, nil, &protocol.Error{Code: protocol.CodeInvalidQueryType, Message: fmt.Errorf("unable to query for type, %s (%d)", q.Type.Name(), q.Type.AsUint64())}
	}
	return k, v, err
}

func (m *Executor) expandChainEntries(entries []string) ([]*state.Object, error) {
	expEntries := make([]*state.Object, len(entries))
	for i, entry := range entries {
		index := i
		r, err := m.expandChainEntry(entry)
		if err != nil {
			return nil, err
		}
		expEntries[index] = r
	}
	return expEntries, nil
}

func (m *Executor) expandChainEntry(entryUrl string) (*state.Object, error) {
	u, err := url.Parse(entryUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid URL in query %s", entryUrl)
	}

	v, err := m.queryByChainId(u.ResourceChain())
	if err != nil {
		return nil, &protocol.Error{Code: protocol.CodeTxnQueryError, Message: err}
	}
	return &state.Object{Entry: v.Entry, Height: v.Height, Roots: v.Roots}, nil
}

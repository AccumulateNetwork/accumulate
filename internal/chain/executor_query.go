package chain

import (
	"encoding"
	"encoding/hex"
	"errors"
	"fmt"

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

func (m *Executor) queryDirectoryByChainId(chainId []byte) (*protocol.DirectoryQueryResult, error) {
	b, err := m.DB.GetIndex(state.DirectoryIndex, chainId, "Metadata")
	if err != nil {
		return nil, err
	}

	md := new(protocol.DirectoryIndexMetadata)
	err = md.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}

	resp := new(protocol.DirectoryQueryResult)
	resp.Entries = make([]string, md.Count)
	for i := range resp.Entries {
		b, err := m.DB.GetIndex(state.DirectoryIndex, chainId, uint64(i))
		if err != nil {
			return nil, fmt.Errorf("failed to get entry %d", i)
		}
		resp.Entries[i] = string(b)
	}
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

	qr.TxId.FromBytes(txid)

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
		dir, err := m.queryDirectoryByChainId(u.ResourceChain())
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.CodeDirectoryURL, Message: err}
		}

		if chr.ExpandChains {
			entries, err := m.expandChainEntries(dir.Entries)
			if err != nil {
				return nil, nil, &protocol.Error{Code: protocol.CodeDirectoryURL, Message: err}
			}
			dir.ExpandedEntries = entries
			dir.Entries = nil
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

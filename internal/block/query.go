package block

import (
	"encoding"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

func (m *Executor) queryAccount(batch *database.Batch, account *database.Account, prove bool) (*query.ResponseAccount, error) {
	resp := new(query.ResponseAccount)

	state, err := account.GetState()
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}
	resp.Account = state

	obj, err := account.GetObject()
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}

	for _, c := range obj.Chains {
		chain, err := account.ReadChain(c.Name)
		if err != nil {
			return nil, fmt.Errorf("get chain %s: %w", c.Name, err)
		}

		ms := chain.CurrentState()
		var state query.ChainState
		state.Name = c.Name
		state.Type = c.Type
		state.Height = uint64(ms.Count)
		state.Roots = make([][]byte, len(ms.Pending))
		for i, h := range ms.Pending {
			state.Roots[i] = h
		}

		resp.ChainState = append(resp.ChainState, state)
	}

	if !prove {
		return resp, nil
	}

	resp.Receipt, err = m.resolveAccountStateReceipt(batch, account)
	if err != nil {
		resp.Receipt.Error = err.Error()
	}

	return resp, nil
}

func (m *Executor) queryByUrl(batch *database.Batch, u *url.URL, prove bool) ([]byte, encoding.BinaryMarshaler, error) {
	qv := u.QueryValues()

	switch {
	case qv.Get("txid") != "":
		// Query by transaction ID
		txid, err := hex.DecodeString(qv.Get("txid"))
		if err != nil {
			return nil, nil, fmt.Errorf("invalid txid %q: %v", qv.Get("txid"), err)
		}

		v, err := m.queryByTxId(batch, txid, prove, false)
		return []byte("tx"), v, err

	case u.Fragment == "":
		// Query by account URL
		account, err := m.queryAccount(batch, batch.Account(u), prove)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to load %v: %w", u, err)
		}
		return []byte("account"), account, err
	}

	fragment := strings.Split(u.Fragment, "/")
	switch fragment[0] {
	case "anchor":
		if len(fragment) < 2 {
			return nil, nil, fmt.Errorf("invalid fragment")
		}

		entryHash, err := hex.DecodeString(fragment[1])
		if err != nil {
			return nil, nil, fmt.Errorf("invalid entry: %q is not a hash", fragment[1])
		}

		obj, err := batch.Account(u).GetObject()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load metadata of %q: %v", u, err)
		}

		var chainName string
		var index int64
		for _, chainMeta := range obj.Chains {
			if chainMeta.Type != protocol.ChainTypeAnchor {
				continue
			}

			chain, err := batch.Account(u).ReadChain(chainMeta.Name)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to load chain %s of %q: %v", chainMeta.Name, u, err)
			}

			index, err = chain.HeightOf(entryHash)
			if err == nil {
				chainName = chainMeta.Name
				break
			}
			if !errors.Is(err, storage.ErrNotFound) {
				return nil, nil, err
			}
		}
		if chainName == "" {
			return nil, nil, errors.NotFound("anchor %X not found", entryHash[:4])
		}
		res := new(query.ResponseChainEntry)
		res.Type = protocol.ChainTypeAnchor
		res.Height = index
		res.Entry = entryHash

		res.Receipt, err = m.resolveChainReceipt(batch, u, chainName, index)
		if err != nil {
			res.Receipt.Error = err.Error()
		}

		return []byte("chain-entry"), res, nil

	case "chain":
		if len(fragment) < 2 {
			return nil, nil, fmt.Errorf("invalid fragment")
		}

		obj, err := batch.Account(u).GetObject()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load metadata of %q: %v", u, err)
		}

		chain, err := batch.Account(u).ReadChain(fragment[1])
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load chain %q of %q: %v", strings.Join(fragment[1:], "."), u, err)
		}

		switch len(fragment) {
		case 2:
			start, count, err := parseRange(qv)
			if err != nil {
				return nil, nil, err
			}

			res := new(query.ResponseChainRange)
			res.Type = obj.ChainType(fragment[1])
			res.Start = start
			res.End = start + count
			res.Total = chain.Height()
			res.Entries, err = chain.Entries(start, start+count)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to load entries: %v", err)
			}

			return []byte("chain-range"), res, nil

		case 3:
			height, entry, err := getChainEntry(chain, fragment[2])
			if err != nil {
				return nil, nil, err
			}

			state, err := chain.State(height)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to load chain state: %v", err)
			}

			res := new(query.ResponseChainEntry)
			res.Type = obj.ChainType(fragment[1])
			res.Height = height
			res.Entry = entry
			res.State = make([][]byte, len(state.Pending))
			for i, h := range state.Pending {
				res.State[i] = h.Copy()
			}
			return []byte("chain-entry"), res, nil
		}

	case "tx", "txn", "transaction", "signature":
		chainName := chainNameFor(fragment[0])
		switch len(fragment) {
		case 1:
			start, count, err := parseRange(qv)
			if err != nil {
				return nil, nil, err
			}

			txns, perr := m.queryTxHistory(batch, u, uint64(start), uint64(start+count), protocol.MainChain)
			if perr != nil {
				return nil, nil, perr
			}

			return []byte("tx-history"), txns, nil

		case 2:
			chain, err := batch.Account(u).ReadChain(chainName)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to load main chain of %q: %v", u, err)
			}

			height, txid, err := getTransaction(chain, fragment[1])
			if err != nil {
				return nil, nil, err
			}

			state, err := chain.State(height)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to load chain state: %v", err)
			}

			res, err := m.queryByTxId(batch, txid, prove, false)
			if err != nil {
				return nil, nil, err
			}

			res.Height = height
			res.ChainState = make([][]byte, len(state.Pending))
			for i, h := range state.Pending {
				res.ChainState[i] = h.Copy()
			}

			return []byte("tx"), res, nil
		}
	case "pending":
		switch len(fragment) {
		case 1:
			txIds, err := batch.Account(u).Pending()
			if err != nil {
				return nil, nil, err
			}
			resp := new(query.ResponsePending)
			resp.Transactions = txIds
			return []byte("pending"), resp, nil
		case 2:
			if strings.Contains(fragment[1], ":") {
				indexes := strings.Split(fragment[1], ":")
				start, err := strconv.Atoi(indexes[0])
				if err != nil {
					return nil, nil, err
				}
				end, err := strconv.Atoi(indexes[1])
				if err != nil {
					return nil, nil, err
				}
				txns, perr := m.queryTxHistory(batch, u, uint64(start), uint64(end), protocol.SignatureChain)
				if perr != nil {
					return nil, nil, perr
				}
				return []byte("tx-history"), txns, nil
			} else {
				chain, err := batch.Account(u).ReadChain(protocol.SignatureChain)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to load main chain of %q: %v", u, err)
				}

				height, txid, err := getTransaction(chain, fragment[1])
				if err != nil {
					return nil, nil, err
				}

				state, err := chain.State(height)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to load chain state: %v", err)
				}

				res, err := m.queryByTxId(batch, txid, prove, false)
				if err != nil {
					return nil, nil, err
				}

				res.Height = height
				res.ChainState = make([][]byte, len(state.Pending))
				for i, h := range state.Pending {
					res.ChainState[i] = h.Copy()
				}

				return []byte("tx"), res, nil
			}
		}
	case "data":
		switch len(fragment) {
		case 1:
			res, err := m.queryDataByUrl(batch, u)
			if err != nil {
				return nil, nil, err
			}
			return []byte("data-entry"), res, nil
		case 2:
			queryParam := fragment[1]
			if strings.Contains(queryParam, ":") {
				indexes := strings.Split(queryParam, ":")
				start, err := strconv.Atoi(indexes[0])
				if err != nil {
					return nil, nil, err
				}
				end, err := strconv.Atoi(indexes[1])
				if err != nil {
					return nil, nil, err
				}
				res, err := m.queryDataSet(batch, u, int64(start), int64(end-start), true)
				if err != nil {
					return nil, nil, err
				}
				return []byte("data-entry-set"), res, nil
			} else {
				index, err := strconv.Atoi(queryParam)
				if err != nil {
					entryHash, err := hex.DecodeString(queryParam)
					if err != nil {
						return nil, nil, err
					}
					res, err := m.queryDataByEntryHash(batch, u, entryHash)
					if err != nil {
						return nil, nil, err
					}
					return []byte("data-entry"), res, nil
				} else {
					entryHash, err := indexing.Data(batch, u).Entry(uint64(index))
					if err != nil {
						return nil, nil, err
					}

					txnHash, err := indexing.Data(batch, u).Transaction(entryHash)
					if err != nil {
						return nil, nil, err
					}

					entry, err := indexing.GetDataEntry(batch, txnHash)
					if err != nil {
						return nil, nil, err
					}

					res := &query.ResponseDataEntry{}
					res.EntryHash = *(*[32]byte)(entryHash)
					res.Entry = entry
					return []byte("data-entry"), res, nil
				}
			}
		}
	}
	return nil, nil, fmt.Errorf("invalid fragment")
}

func chainNameFor(entity string) string {
	switch entity {
	case "signature":
		return protocol.SignatureChain
	}
	return protocol.MainChain
}

func parseRange(qv url.Values) (start, count int64, err error) {
	if s := qv.Get("start"); s != "" {
		start, err = strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid start: %v", err)
		}
	} else {
		start = 0
	}

	if s := qv.Get("count"); s != "" {
		count, err = strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid count: %v", err)
		}
	} else {
		count = 10
	}

	return start, count, nil
}

func getChainEntry(chain *database.Chain, s string) (int64, []byte, error) {
	var valid bool

	height, err := strconv.ParseInt(s, 10, 64)
	if err == nil {
		valid = true
		entry, err := chain.Entry(height)
		if err == nil {
			return height, entry, nil
		}
		if !errors.Is(err, storage.ErrNotFound) {
			return 0, nil, err
		}
	}

	entry, err := hex.DecodeString(s)
	if err == nil {
		valid = true
		height, err := chain.HeightOf(entry)
		if err == nil {
			return height, entry, nil
		}
		if !errors.Is(err, storage.ErrNotFound) {
			return 0, nil, err
		}
	}

	if valid {
		return 0, nil, errors.NotFound("entry %q not found", s)
	}
	return 0, nil, fmt.Errorf("invalid entry: %q is not a number or a hash", s)
}

func getTransaction(chain *database.Chain, s string) (int64, []byte, error) {
	var valid bool

	height, err := strconv.ParseInt(s, 10, 64)
	if err == nil {
		valid = true
		id, err := chain.Entry(height)
		if err == nil {
			return height, id, nil
		}
		if !errors.Is(err, storage.ErrNotFound) {
			return 0, nil, err
		}
	}

	entry, err := hex.DecodeString(s)
	if err == nil {
		valid = true
		height, err := chain.HeightOf(entry)
		if err == nil {
			return height, entry, nil
		}
		if !errors.Is(err, storage.ErrNotFound) {
			return 0, nil, err
		}
	}

	if valid {
		return 0, nil, errors.NotFound("transaction %q not found", s)
	}
	return 0, nil, fmt.Errorf("invalid transaction: %q is not a number or a hash", s)
}

func (m *Executor) queryDirectoryByChainId(batch *database.Batch, account *url.URL, start uint64, limit uint64) (*query.DirectoryQueryResult, error) {
	dir := indexing.Directory(batch, account)
	mdCount, err := dir.Count()
	if err != nil {
		return nil, err
	}

	count := limit
	if start+count > mdCount {
		count = mdCount - start
	}
	if count > mdCount { // when uint64 0-x is really big number
		count = 0
	}

	resp := new(query.DirectoryQueryResult)
	resp.Entries = make([]string, count)

	for i := uint64(0); i < count; i++ {
		u, err := dir.Get(start + i)
		if err != nil {
			return nil, fmt.Errorf("failed to get entry %d", i)
		}
		resp.Entries[i] = u.String()
	}
	resp.Total = mdCount

	return resp, nil
}

func (m *Executor) queryByTxId(batch *database.Batch, txid []byte, prove, remote bool) (*query.ResponseByTxId, error) {
	var err error

	tx := batch.Transaction(txid)
	txState, err := tx.GetState()
	if errors.Is(err, storage.ErrNotFound) {
		return nil, errors.NotFound("transaction %X not found", txid[:4])
	} else if err != nil {
		return nil, fmt.Errorf("invalid query from GetTx in state database, %v", err)
	}

	if txState.Transaction == nil {
		tx = batch.Transaction(txState.Hash[:])
		txState, err = tx.GetState()
		if errors.Is(err, storage.ErrNotFound) {
			return nil, errors.NotFound("transaction not found for signature %X", txid[:4])
		} else if err != nil {
			return nil, fmt.Errorf("invalid query from GetTx in state database, %v", err)
		}
	}

	status, err := tx.GetStatus()
	if err != nil {
		return nil, fmt.Errorf("invalid query from GetTx in state database, %v", err)
	} else if !remote && !status.Delivered && status.Remote {
		// If the transaction is a synthetic transaction produced by this BVN
		// and has not been delivered, pretend like it doesn't exist
		return nil, errors.NotFound("transaction %X not found", txid[:4])
	}

	// If we have an account, lookup if it's a scratch chain. If so, filter out records that should have been pruned
	account := txState.Transaction.Header.Principal
	if account != nil && isScratchAccount(batch, account) {
		shouldBePruned, err := m.shouldBePruned(batch, txid)
		if err != nil {
			return nil, err
		}
		if shouldBePruned {
			return nil, errors.NotFound("transaction %X not found", txid[:4])
		}
	}

	qr := query.ResponseByTxId{}
	qr.Envelope = new(protocol.Envelope)
	qr.Envelope.Transaction = []*protocol.Transaction{txState.Transaction}
	qr.Status = status
	copy(qr.TxId[:], txid)
	qr.Height = -1

	synth, err := tx.GetSyntheticTxns()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("invalid query from GetTx in state database, %v", err)
	}

	qr.TxSynthTxIds = make(types.Bytes, 0, len(synth.Hashes)*32)
	for _, synth := range synth.Hashes {
		synth := synth // See docs/developer/rangevarref.md
		qr.TxSynthTxIds = append(qr.TxSynthTxIds, synth[:]...)
	}

	for _, signer := range status.Signers {
		// Load the signature set
		sigset, err := tx.ReadSignaturesForSigner(signer)
		if err != nil {
			return nil, err
		}

		// Load all the signatures
		var qset query.SignatureSet
		qset.Account = signer
		for _, e := range sigset.Entries() {
			state, err := batch.Transaction(e.SignatureHash[:]).GetState()
			switch {
			case err == nil:
				qset.Signatures = append(qset.Signatures, state.Signature)
			case errors.Is(err, storage.ErrNotFound):
				m.logger.Info("Signature not found", "txn-hash", logging.AsHex(txid).Slice(0, 4), "sig-hash", logging.AsHex(e.SignatureHash).Slice(0, 4), "signer", signer.GetUrl())
				// Leave it nil
			default:
				return nil, fmt.Errorf("load signature entry %X: %w", e.SignatureHash, err)
			}
		}

		qr.Signers = append(qr.Signers, qset)
	}

	if !prove {
		return &qr, nil
	}

	chainIndex, err := indexing.TransactionChain(batch, txid).Get()
	if err != nil {
		return nil, fmt.Errorf("failed to load transaction chain index: %v", err)
	}

	qr.Receipts = make([]*query.TxReceipt, len(chainIndex.Entries))
	for i, entry := range chainIndex.Entries {
		receipt, err := m.resolveTxReceipt(batch, txid, entry)
		if err != nil {
			// If one receipt fails to build, do not cause the entire request to
			// fail
			receipt.Error = err.Error()
		}
		qr.Receipts[i] = receipt
	}

	return &qr, nil
}

func (m *Executor) queryTxHistory(batch *database.Batch, account *url.URL, start, end uint64, chainName string) (*query.ResponseTxHistory, *protocol.Error) {
	chain, err := batch.Account(account).ReadChain(chainName)
	if err != nil {
		return nil, &protocol.Error{Code: protocol.ErrorCodeTxnHistory, Message: fmt.Errorf("error obtaining txid range %v", err)}
	}

	thr := query.ResponseTxHistory{}
	thr.Start = start
	thr.End = end
	thr.Total = uint64(chain.Height())

	txids, err := chain.Entries(int64(start), int64(end))
	if err != nil {
		return nil, &protocol.Error{Code: protocol.ErrorCodeTxnHistory, Message: fmt.Errorf("error obtaining txid range %v", err)}
	}

	for _, txid := range txids {
		qr, err := m.queryByTxId(batch, txid, false, false)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				continue // txs can be filtered out for scratch accounts
			}
			return nil, &protocol.Error{Code: protocol.ErrorCodeTxnQueryError, Message: err}
		}
		thr.Transactions = append(thr.Transactions, *qr)
	}

	return &thr, nil
}

func (m *Executor) queryDataByUrl(batch *database.Batch, u *url.URL) (*query.ResponseDataEntry, error) {
	qr := query.ResponseDataEntry{}

	_, entryHash, txnHash, err := indexing.Data(batch, u).GetLatest()
	if err != nil {
		return nil, err
	}

	qr.Entry, err = indexing.GetDataEntry(batch, txnHash)
	if err != nil {
		return nil, err
	}

	copy(qr.EntryHash[:], entryHash)
	return &qr, nil
}

func (m *Executor) queryDataByEntryHash(batch *database.Batch, u *url.URL, entryHash []byte) (*query.ResponseDataEntry, error) {
	qr := query.ResponseDataEntry{}
	copy(qr.EntryHash[:], entryHash)

	txnHash, err := indexing.Data(batch, u).Transaction(entryHash)
	if err != nil {
		return nil, err
	}

	qr.Entry, err = indexing.GetDataEntry(batch, txnHash)
	if err != nil {
		return nil, err
	}

	return &qr, nil
}

func (m *Executor) queryDataSet(batch *database.Batch, u *url.URL, start int64, limit int64, expand bool) (*query.ResponseDataEntrySet, error) {
	data := indexing.Data(batch, u)
	count, err := data.Count()
	if err != nil {
		return nil, err
	}

	qr := query.ResponseDataEntrySet{}
	qr.Total = count

	for i := uint64(0); i < uint64(limit) && i+uint64(start) < count; i++ {
		entryHash, err := data.Entry(uint64(start) + i)
		if err != nil {
			return nil, err
		}

		er := query.ResponseDataEntry{}
		copy(er.EntryHash[:], entryHash)

		if expand {
			txnHash, err := data.Transaction(entryHash)
			if err != nil {
				return nil, err
			}

			er.Entry, err = indexing.GetDataEntry(batch, txnHash)
			if err != nil {
				return nil, err
			}
		}

		qr.DataEntries = append(qr.DataEntries, er)
	}

	return &qr, nil
}

func (m *Executor) Query(batch *database.Batch, q query.Request, _ int64, prove bool) (k, v []byte, _ *protocol.Error) {
	switch q := q.(type) {
	case *query.RequestByTxId:
		txr := q
		qr, err := m.queryByTxId(batch, txr.TxId[:], prove, false)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeTxnQueryError, Message: err}
		}

		k = []byte("tx")
		v, err = qr.MarshalBinary()
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeMarshallingError, Message: fmt.Errorf("%v, on Chain %x", err, txr.TxId[:])}
		}
	case *query.RequestTxHistory:
		txh := q

		thr, perr := m.queryTxHistory(batch, txh.Account, txh.Start, txh.Start+txh.Limit, protocol.MainChain)
		if perr != nil {
			return nil, nil, perr
		}

		var err error
		k = []byte("tx-history")
		v, err = thr.MarshalBinary()
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeMarshallingError, Message: fmt.Errorf("error marshalling payload for transaction history")}
		}
	case *query.RequestByUrl:
		chr := q

		var err error
		var obj encoding.BinaryMarshaler
		k, obj, err = m.queryByUrl(batch, chr.Url, prove)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeTxnQueryError, Message: err}
		}
		v, err = obj.MarshalBinary()
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeMarshallingError, Message: fmt.Errorf("%v, on Url %s", err, chr.Url)}
		}
	case *query.RequestDirectory:
		chr := q
		dir, err := m.queryDirectoryByChainId(batch, chr.Url, chr.Start, chr.Limit)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeDirectoryURL, Message: err}
		}

		if chr.ExpandChains {
			entries, err := m.expandChainEntries(batch, dir.Entries)
			if err != nil {
				return nil, nil, &protocol.Error{Code: protocol.ErrorCodeDirectoryURL, Message: err}
			}
			dir.ExpandedEntries = entries
		}

		k = []byte("directory")
		v, err = dir.MarshalBinary()
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeMarshallingError, Message: fmt.Errorf("%v, on Url %s", err, chr.Url)}
		}
	case *query.RequestByChainId:
		chr := q

		//nolint:staticcheck // Ignore the deprecation warning for AccountByID
		record, err := batch.AccountByID(chr.ChainId[:])
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeChainIdError, Message: err}
		}
		account, err := m.queryAccount(batch, record, false)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeChainIdError, Message: err}
		}
		k = []byte("account")
		v, err = account.MarshalBinary()
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeMarshallingError, Message: fmt.Errorf("%v, on Chain %x", err, chr.ChainId)}
		}
	case *query.RequestDataEntry:
		chr := q

		var err error
		u := chr.Url
		var ret *query.ResponseDataEntry
		if chr.EntryHash != [32]byte{} {
			ret, err = m.queryDataByEntryHash(batch, u, chr.EntryHash[:])
			if err != nil {
				return nil, nil, &protocol.Error{Code: protocol.ErrorCodeDataEntryHashError, Message: err}
			}
		} else {
			ret, err = m.queryDataByUrl(batch, u)
			if err != nil {
				return nil, nil, &protocol.Error{Code: protocol.ErrorCodeDataUrlError, Message: err}
			}
		}

		k = []byte("data")
		v, err = ret.MarshalBinary()
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeMarshallingError, Message: err}
		}
	case *query.RequestDataEntrySet:
		chr := q
		u := chr.Url
		ret, err := m.queryDataSet(batch, u, int64(chr.Start), int64(chr.Count), chr.ExpandChains)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeDataEntryHashError, Message: err}
		}

		k = []byte("dataSet")
		v, err = ret.MarshalBinary()
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeMarshallingError, Message: err}
		}
	case *query.RequestKeyPageIndex:
		chr := q
		account, err := batch.Account(chr.Url).GetState()
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeChainIdError, Message: err}
		}

		auth, err := m.GetAccountAuthoritySet(batch, account)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeChainIdError, Message: err}
		}

		// For each authority
		for _, entry := range auth.Authorities {
			var authority protocol.Authority
			err = batch.Account(entry.Url).GetStateAs(&authority)
			if err != nil {
				return nil, nil, protocol.NewError(protocol.ErrorCodeUnknownError, err)
			}

			// For each signer
			for index, signerUrl := range authority.GetSigners() {
				var signer protocol.Signer
				err = batch.Account(signerUrl).GetStateAs(&signer)
				if err != nil {
					return nil, nil, protocol.NewError(protocol.ErrorCodeUnknownError, err)
				}

				// Check for a matching entry
				_, _, ok := signer.EntryByKeyHash(chr.Key)
				if !ok {
					_, _, ok = signer.EntryByKey(chr.Key)
					if !ok {
						continue
					}
				}

				// Found it!
				response := new(query.ResponseKeyPageIndex)
				response.Authority = entry.Url
				response.Signer = signerUrl
				response.Index = uint64(index)

				k = []byte("key-page-index")
				v, err = response.MarshalBinary()
				if err != nil {
					return nil, nil, &protocol.Error{Code: protocol.ErrorCodeMarshallingError, Message: err}
				}
				return k, v, nil
			}
		}
		return nil, nil, &protocol.Error{Code: protocol.ErrorCodeNotFound, Message: fmt.Errorf("no authority of %s holds %X", chr.Url, chr.Key)}
	case *query.RequestMinorBlocksByUrl:
		resp, pErr := m.queryMinorBlocksByUrl(batch, q)
		if pErr != nil {
			return nil, nil, pErr
		}

		k = []byte("minor-block")
		var err error
		v, err = resp.MarshalBinary()
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeMarshallingError, Message: fmt.Errorf("error marshalling payload for transaction history")}
		}
	case *query.RequestSynth:
		subnet, ok := protocol.ParseSubnetUrl(q.Destination)
		if !ok {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeInvalidRequest, Message: fmt.Errorf("destination is not a subnet")}
		}
		record := batch.Account(m.Network.Synthetic())
		chain, err := record.ReadChain(protocol.SyntheticIndexChain(subnet))
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeInternal, Message: fmt.Errorf("failed to load the synth index chain: %w", err)}
		}
		entry := new(protocol.IndexEntry)
		err = chain.EntryAs(int64(q.SequenceNumber)-1, entry)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeInternal, Message: fmt.Errorf("failed to unmarshal the index entry: %w", err)}
		}
		chain, err = record.ReadChain(protocol.MainChain)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeInternal, Message: fmt.Errorf("failed to load the synth main chain: %w", err)}
		}
		hash, err := chain.Entry(int64(entry.Source))
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeInternal, Message: fmt.Errorf("failed to load the main entry: %w", err)}
		}
		qr, err := m.queryByTxId(batch, hash, prove, true)
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeTxnQueryError, Message: err}
		}

		keySig, err := new(signing.Builder).
			SetType(protocol.SignatureTypeED25519).
			SetPrivateKey(m.Key).
			SetKeyPageUrl(m.Network.ValidatorBook(), 0).
			SetVersion(1).
			SetTimestamp(1).
			Sign(hash)
		if err != nil {
			return nil, nil, protocol.Errorf(protocol.ErrorCodeInternal, "sign synthetic transaction: %w", err)
		}

		// This is a hack
		qr.Envelope.Signatures = append(qr.Envelope.Signatures, keySig)
		qr.Signers = append(qr.Signers, query.SignatureSet{
			Account:    &protocol.UnknownAccount{Url: keySig.GetSigner()},
			Signatures: []protocol.Signature{keySig},
		})

		k = []byte("tx")
		v, err = qr.MarshalBinary()
		if err != nil {
			return nil, nil, &protocol.Error{Code: protocol.ErrorCodeMarshallingError, Message: fmt.Errorf("%v, on Chain %x", err, hash)}
		}

	default:
		return nil, nil, &protocol.Error{Code: protocol.ErrorCodeInvalidQueryType, Message: fmt.Errorf("unable to query for type, %s (%d)", q.Type().String(), q.Type().GetEnumValue())}
	}
	return k, v, nil
}

func (m *Executor) resolveTxReceipt(batch *database.Batch, txid []byte, entry *indexing.TransactionChainEntry) (*query.TxReceipt, error) {
	receipt := new(query.TxReceipt)
	receipt.Account = entry.Account
	receipt.Chain = entry.Chain
	receipt.Proof.Start = txid

	account := batch.Account(entry.Account)
	block, r, err := indexing.ReceiptForChainEntry(&m.Network, batch, account, txid, entry)
	if err != nil {
		return receipt, err
	}

	receipt.LocalBlock = block
	receipt.Proof = *protocol.ReceiptFromManaged(r)
	return receipt, nil
}

func (m *Executor) resolveChainReceipt(batch *database.Batch, account *url.URL, name string, index int64) (*query.GeneralReceipt, error) {
	receipt := new(query.GeneralReceipt)
	_, r, err := indexing.ReceiptForChainIndex(&m.Network, batch, batch.Account(account), name, index)
	if err != nil {
		return receipt, err
	}

	receipt.Proof = *protocol.ReceiptFromManaged(r)
	return receipt, nil
}

func (m *Executor) resolveAccountStateReceipt(batch *database.Batch, account *database.Account) (*query.GeneralReceipt, error) {
	receipt := new(query.GeneralReceipt)
	block, r, err := indexing.ReceiptForAccountState(&m.Network, batch, account)
	if err != nil {
		return receipt, err
	}

	receipt.LocalBlock = block
	receipt.Proof = *protocol.ReceiptFromManaged(r)
	return receipt, nil
}

func isScratchAccount(batch *database.Batch, account *url.URL) bool {
	acc := batch.Account(account)
	state, err := acc.GetState()
	if err != nil {
		return false // Account may not exist, don't emit an error because waitForTxns will not get back the tx for this BVN and fail
	}

	switch v := state.(type) {
	case *protocol.DataAccount:
		return v.Scratch
	case *protocol.TokenAccount:
		return v.Scratch
	}
	return false
}

func (m *Executor) shouldBePruned(batch *database.Batch, txid []byte) (bool, error) {

	// Load the tx chain
	txChain, err := indexing.TransactionChain(batch, txid).Get()
	if err != nil {
		return false, err
	}

	pruneTime := time.Now().AddDate(0, 0, 0-protocol.ScratchPrunePeriodDays)

	// preload the minor root index chain
	ledger := batch.Account(m.Network.NodeUrl(protocol.Ledger))
	minorIndexChain, err := ledger.ReadChain(protocol.MinorRootIndexChain)
	if err != nil {
		return false, err
	}

	for _, txChainEntry := range txChain.Entries {
		if txChainEntry.Chain == protocol.MainChain {
			// Load the index entry
			indexEntry := new(protocol.IndexEntry)
			err = minorIndexChain.EntryAs(int64(txChainEntry.AnchorIndex), indexEntry)
			if err != nil {
				return false, err
			}
			if indexEntry.BlockTime.Before(pruneTime) {
				return true, nil
			}
			return false, nil
		}
	}
	return false, nil
}

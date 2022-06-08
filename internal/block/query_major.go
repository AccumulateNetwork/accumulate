package block

import (
	"errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

func (m *Executor) queryMajorBlocks(batch *database.Batch, req *query.RequestMajorBlocks) (resp *query.ResponseMajorBlocks, perr *protocol.Error) {

	anchorsAcc := batch.Account(m.Network.NodeUrl(protocol.AnchorPool))
	ledgerAcc := batch.Account(m.Network.NodeUrl(protocol.Ledger))

	mjrIdxChain, err := anchorsAcc.ReadChain(protocol.IndexChain(protocol.MainChain, true))
	if err != nil {
		return nil, &protocol.Error{Code: protocol.ErrorCodeQueryChainUpdatesError, Message: err}
	}

	if req.Start == 0 { // We don't have major block 0, avoid crash
		req.Start = 1
	}
	mjrStartIdx, _, err := indexing.SearchIndexChain(mjrIdxChain, uint64(mjrIdxChain.Height())-1, indexing.MatchAfter, indexing.SearchIndexChainByBlock(req.Start))
	if err != nil {
		return nil, &protocol.Error{Code: protocol.ErrorCodeQueryEntriesError, Message: err}
	}

	resp = &query.ResponseMajorBlocks{TotalBlocks: uint64(mjrIdxChain.Height())}
	curEntry := new(protocol.IndexEntry)
	resultCnt := uint64(0)
	mjrEntryIdx := mjrStartIdx
	mnrStartIdx := uint64(1)

	if mjrEntryIdx > 0 {
		mnrStartIdx, perr = getPrevEntryRootIndex(mjrIdxChain, mjrEntryIdx)
		if err != nil {
			return nil, perr
		}
	}

majorEntryLoop:
	for resultCnt < req.Limit {
		err = mjrIdxChain.EntryAs(int64(mjrEntryIdx), curEntry)
		switch {
		case err == nil:
		case errors.Is(err, storage.ErrNotFound):
			break majorEntryLoop
		default:
			return nil, &protocol.Error{Code: protocol.ErrorCodeUnMarshallingError, Message: err}
		}

		rspMjrEntry := new(query.ResponseMajorEntry)
		for {
			rspMjrEntry.MajorBlockIndex = req.Start + resultCnt
			if rspMjrEntry.MajorBlockIndex >= curEntry.BlockIndex && curEntry.BlockIndex != 0 {
				break
			}

			// Append empty entry when blocks were missing
			resp.Entries = append(resp.Entries, rspMjrEntry)
			resultCnt++
			rspMjrEntry = new(query.ResponseMajorEntry)
		}

		mnrIdxChain, err := ledgerAcc.ReadChain(protocol.MinorRootIndexChain)
		if err != nil {
			return nil, &protocol.Error{Code: protocol.ErrorCodeQueryChainUpdatesError, Message: err}
		}

		mnrIdx, mnrIdxEntry, err := indexing.SearchIndexChain(mnrIdxChain, uint64(mnrIdxChain.Height())-1, indexing.MatchAfter, indexing.SearchIndexChainByBlock(mnrStartIdx))
		if err != nil {
			return nil, &protocol.Error{Code: protocol.ErrorCodeQueryEntriesError, Message: err}
		}

	minorEntryLoop:
		for {
			rspMnrEntry := new(query.ResponseMinorEntry)
			err = mnrIdxChain.EntryAs(int64(mnrIdx), mnrIdxEntry)
			switch {
			case err == nil:
			case errors.Is(err, storage.ErrNotFound):
				break minorEntryLoop
			default:
				return nil, &protocol.Error{Code: protocol.ErrorCodeUnMarshallingError, Message: err}
			}
			if mnrIdxEntry.BlockIndex > curEntry.RootIndexIndex {
				break minorEntryLoop
			}
			rspMnrEntry.BlockIndex = mnrIdxEntry.BlockIndex
			rspMnrEntry.BlockTime = mnrIdxEntry.BlockTime
			rspMjrEntry.MinorBlocks = append(rspMjrEntry.MinorBlocks, rspMnrEntry)
			mnrIdx++
		}
		rspMjrEntry.MajorBlockTime = curEntry.BlockTime
		resp.Entries = append(resp.Entries, rspMjrEntry)
		mnrStartIdx = mnrIdxEntry.BlockIndex
		mjrEntryIdx++
		resultCnt++
	}
	return resp, nil
}

func getPrevEntryRootIndex(mjrIdxChain *database.Chain, mjrEntryIdx uint64) (uint64, *protocol.Error) {
	prevEntry := new(protocol.IndexEntry)
	err := mjrIdxChain.EntryAs(int64(mjrEntryIdx)-1, prevEntry)
	if err != nil {
		return 0, &protocol.Error{Code: protocol.ErrorCodeUnMarshallingError, Message: err}
	}
	return prevEntry.RootIndexIndex + 1, nil
}

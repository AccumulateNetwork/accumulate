package block

import (
	"errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

func (m *Executor) queryMajorBlocks(batch *database.Batch, req *query.RequestMajorBlocks) (*query.ResponseMajorBlocks, *protocol.Error) {
	anchorsAcc := batch.Account(m.Network.NodeUrl(protocol.AnchorPool))
	var anchorLedger *protocol.AnchorLedger
	err := anchorsAcc.GetStateAs(&anchorLedger)
	if err != nil {
		return nil, &protocol.Error{Code: protocol.ErrorCodeUnMarshallingError, Message: err}
	}
	ledgerAcc := batch.Account(m.Network.NodeUrl(protocol.Ledger))
	var systemLedger *protocol.SystemLedger
	err = ledgerAcc.GetStateAs(&systemLedger)
	if err != nil {
		return nil, &protocol.Error{Code: protocol.ErrorCodeUnMarshallingError, Message: err}
	}

	mjrIdxChain, err := anchorsAcc.ReadChain(protocol.IndexChain(protocol.MainChain, true))
	if err != nil {
		return nil, &protocol.Error{Code: protocol.ErrorCodeQueryChainUpdatesError, Message: err}
	}

	mjrStartIdx, _, err := indexing.SearchIndexChain(mjrIdxChain, uint64(mjrIdxChain.Height())-1, indexing.MatchAfter, indexing.SearchIndexChainByBlock(req.Start))
	if err != nil {
		return nil, &protocol.Error{Code: protocol.ErrorCodeQueryEntriesError, Message: err}
	}

	mjrEntryIdx := mjrStartIdx

	resp := query.ResponseMajorBlocks{TotalBlocks: uint64(mjrIdxChain.Height())}
	curEntry := new(protocol.IndexEntry)
	resultCnt := uint64(0)

	firstBlock := false
	prevEntry := new(protocol.IndexEntry)
	err = mjrIdxChain.EntryAs(int64(mjrEntryIdx), prevEntry)
	switch {
	case err == nil:
	case errors.Is(err, storage.ErrNotFound):
		return &resp, nil
	default:
		return nil, &protocol.Error{Code: protocol.ErrorCodeUnMarshallingError, Message: err}
	}
	mjrEntryIdx++

	for resultCnt < req.Limit && !firstBlock {
		err = mjrIdxChain.EntryAs(int64(mjrEntryIdx), curEntry)
		switch {
		case err == nil:
		case errors.Is(err, storage.ErrNotFound):
			firstBlock = true
		default:
			return nil, &protocol.Error{Code: protocol.ErrorCodeUnMarshallingError, Message: err}
		}

		mjrEntry := new(query.ResponseMajorEntry)
		for {
			mjrEntry.MajorBlockIndex = req.Start + resultCnt

			// Create new entry, append empty entry when blocks were missing
			if mjrEntry.MajorBlockIndex < curEntry.BlockIndex || curEntry.BlockIndex == 0 {
				resp.Entries = append(resp.Entries, mjrEntry)
				resultCnt++
				mjrEntry = new(query.ResponseMajorEntry)
				continue
			}
			break
		}
		mjrEntry.MajorBlockTime = curEntry.BlockTime

		mnrIdxChain, err := ledgerAcc.ReadChain(protocol.MinorRootIndexChain)
		if err != nil {
			return nil, &protocol.Error{Code: protocol.ErrorCodeQueryChainUpdatesError, Message: err}
		}

		startIdx := uint64(0)
		if !firstBlock {
			startIdx = prevEntry.RootIndexIndex
		}
		mnrIdx, mnrIdxEntry, err := indexing.SearchIndexChain(mnrIdxChain, uint64(mnrIdxChain.Height())-1, indexing.MatchAfter, indexing.SearchIndexChainByBlock(startIdx))
		if err != nil {
			return nil, &protocol.Error{Code: protocol.ErrorCodeQueryEntriesError, Message: err}
		}

	minorEntryLoop:
		for {
			mnrEntry := new(query.ResponseMinorEntry)
			mnrEntry.BlockIndex = mnrIdxEntry.BlockIndex
			mnrEntry.BlockTime = mnrIdxEntry.BlockTime
			err = mnrIdxChain.EntryAs(int64(mnrIdx), mnrIdxEntry)
			switch {
			case err == nil:
			case errors.Is(err, storage.ErrNotFound):
				break minorEntryLoop
			default:
				return nil, &protocol.Error{Code: protocol.ErrorCodeUnMarshallingError, Message: err}
			}
			if mnrIdxEntry.BlockIndex >= curEntry.RootIndexIndex {
				break minorEntryLoop
			}
			mjrEntry.MinorBlocks = append(mjrEntry.MinorBlocks, mnrEntry)
			mnrIdx++
		}
		resp.Entries = append(resp.Entries, mjrEntry)
		mjrEntryIdx++
		resultCnt++
		prevEntry = curEntry
	}
	return &resp, nil
}

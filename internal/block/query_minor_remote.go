package block

import (
	"context"
	"fmt"

	"github.com/tendermint/tendermint/rpc/client"
	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/query"
)

const autoFlushThreshold = 128 // The maximum number of items in the queue before it is flushed and a remote query is executed

type minorBlockRemote struct {
	block uint64
	entry *query.ResponseMinorEntry
}

type minorBlockQueue []minorBlockRemote
type blockTxsMap map[uint64][]*query.ResponseByTxId

type remoteMinorQuerier struct {
	router   routing.Router
	queueMap map[string]*minorBlockQueue
}

type RemoteMinorBlockQuerier interface {
	SubmitQuery(source *url.URL, block uint64, entry *query.ResponseMinorEntry) *protocol.Error
	Flush() *protocol.Error
}

func NewRemoteMinorBlockQuerier(router routing.Router) RemoteMinorBlockQuerier {
	return &remoteMinorQuerier{
		router:   router,
		queueMap: map[string]*minorBlockQueue{},
	}
}

func (m *remoteMinorQuerier) SubmitQuery(source *url.URL, block uint64, entry *query.ResponseMinorEntry) *protocol.Error {
	subnetId, ok := protocol.ParseSubnetUrl(source)
	if !ok {
		return &protocol.Error{Code: protocol.ErrorCodeQueryEntriesError,
			Message: fmt.Errorf("the anchor source URL is %s which is not a BVN", source.String())}
	}

	queueRef, ok := m.queueMap[subnetId]
	if !ok {
		queueRef = &minorBlockQueue{}
		m.queueMap[subnetId] = queueRef
	}
	*queueRef = append(*queueRef, minorBlockRemote{
		block: block,
		entry: entry,
	})
	if len(*queueRef) > autoFlushThreshold {
		err := m.flushQueue(subnetId)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *remoteMinorQuerier) Flush() *protocol.Error {
	for subnetId := range m.queueMap {
		err := m.flushQueue(subnetId)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *remoteMinorQuerier) flushQueue(subnetId string) *protocol.Error {
	queueRef := m.queueMap[subnetId]
	m.queueMap[subnetId] = &minorBlockQueue{}

	ranges := collectRanges(queueRef)
	rmtEntryMap, err := m.queryRemote(subnetId, ranges)
	if err != nil {
		return err
	}
	for _, mbr := range *queueRef {
		entryTxs := rmtEntryMap[mbr.block]
		mbr.entry.Transactions = append(mbr.entry.Transactions, entryTxs...)
		mbr.entry.TxCount += uint64(len(entryTxs))
	}
	return nil
}

// collectRanges loops through the queue and try to create ranges.
// When the TPS count gets higher it's likely most or all BVNs have an anchor in every DN block.
func collectRanges(queueRef *minorBlockQueue) []query.Range {
	var ranges []query.Range
	r := query.Range{}
	for _, mbr := range *queueRef {
		if mbr.block != (r.Start + r.Count) {
			r = query.Range{}
			r.Start = mbr.block
			r.Count = 1
			ranges = append(ranges, r)
		} else {
			r.Count++
		}
	}
	return ranges
}

func (m *remoteMinorQuerier) queryRemote(subnetId string, ranges []query.Range) (blockTxsMap, *protocol.Error) {
	req := &query.RequestMinorBlocks{
		Ranges:          ranges,
		TxFetchMode:     query.TxFetchModeExpand,
		BlockFilterMode: query.BlockFilterModeExcludeNone,
	}
	buf, err := req.MarshalBinary()
	if err != nil {
		return nil, &protocol.Error{Code: protocol.ErrorCodeQueryEntriesError, Message: err}
	}

	res, err := m.router.Query(context.Background(), subnetId, buf, client.ABCIQueryOptions{})
	if err != nil {
		return nil, &protocol.Error{Code: protocol.ErrorCodeQueryEntriesError, Message: err}
	}
	remoteBlocks := new(query.ResponseMinorBlocks)
	err = remoteBlocks.UnmarshalBinary(res.Response.Value)
	if err != nil {
		return nil, &protocol.Error{Code: protocol.ErrorCodeUnMarshallingError, Message: err}
	}

	r := blockTxsMap{}
	for _, entry := range remoteBlocks.Entries {
		r[entry.BlockIndex] = entry.Transactions
	}
	return r, nil
}

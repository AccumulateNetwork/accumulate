// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package headerwalk

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// fakeAnchorQuerier serves a fixed slice of anchor records via the
// Querier interface. Other Query methods return ErrUnsupported so
// surprises surface immediately.
type fakeAnchorQuerier struct {
	scope   *url.URL
	records []*api.ChainEntryRecord[api.Record]
}

func (f *fakeAnchorQuerier) Query(_ context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	if !scope.Equal(f.scope) {
		return nil, errors.New("fake: unexpected scope " + scope.String())
	}
	cq, ok := query.(*api.ChainQuery)
	if !ok {
		return nil, errors.New("fake: only ChainQuery supported")
	}
	if cq.Name != "main" {
		return nil, errors.New("fake: only the main chain is served")
	}

	start := uint64(0)
	count := uint64(len(f.records))
	if cq.Range != nil {
		start = cq.Range.Start
		if cq.Range.Count != nil {
			count = *cq.Range.Count
		}
	}

	end := start + count
	if end > uint64(len(f.records)) {
		end = uint64(len(f.records))
	}
	if start > uint64(len(f.records)) {
		start = uint64(len(f.records))
	}

	page := &api.RecordRange[*api.ChainEntryRecord[api.Record]]{
		Total: uint64(len(f.records)),
		Start: start,
	}
	page.Records = append(page.Records, f.records[start:end]...)
	return page, nil
}

// makeAnchor builds a ChainEntryRecord whose Value is a transaction
// message wrapping a BlockValidatorAnchor at the given height.
func makeAnchor(t *testing.T, partition *url.URL, height uint64, blockTime time.Time, stateRoot [32]byte) *api.ChainEntryRecord[api.Record] {
	t.Helper()
	body := &protocol.BlockValidatorAnchor{
		PartitionAnchor: protocol.PartitionAnchor{
			Source:          partition,
			MinorBlockIndex: height,
			MajorBlockIndex: 0,
			RootChainAnchor: [32]byte{byte(height), 0xaa},
			StateTreeAnchor: stateRoot,
		},
	}
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: partition},
		Body:   body,
	}
	tm := &messaging.TransactionMessage{Transaction: txn}

	mr := &api.MessageRecord[messaging.Message]{
		ID:      txn.ID(),
		Message: tm,
	}
	bt := blockTime
	var hashArr [32]byte
	copy(hashArr[:], txn.GetHash())
	return &api.ChainEntryRecord[api.Record]{
		Account:       partition,
		Name:          "main",
		Type:          0,
		Index:         height - 1,
		Entry:         hashArr,
		Value:         mr,
		LastBlockTime: &bt,
	}
}

func TestAPISource_HeaderResolvesByHeight(t *testing.T) {
	partition := protocol.DnUrl()
	anchorPool := partition.JoinPath(protocol.AnchorPool)

	stateRoot100 := [32]byte{0x10, 0x10}
	stateRoot101 := [32]byte{0x11, 0x11}
	t100 := time.Unix(1700000000, 0).UTC()
	t101 := time.Unix(1700000060, 0).UTC()

	q := &fakeAnchorQuerier{
		scope: anchorPool,
		records: []*api.ChainEntryRecord[api.Record]{
			makeAnchor(t, partition, 100, t100, stateRoot100),
			makeAnchor(t, partition, 101, t101, stateRoot101),
		},
	}

	src := NewAPISource(api.Querier2{Querier: q}, anchorPool)
	hdr, err := src.Header(context.Background(), 100)
	if err != nil {
		t.Fatalf("Header(100): %v", err)
	}
	if hdr.Height != 100 {
		t.Errorf("Height = %d, want 100", hdr.Height)
	}
	if hdr.StateTreeRoot != stateRoot100 {
		t.Errorf("StateTreeRoot mismatch: got %x, want %x", hdr.StateTreeRoot, stateRoot100)
	}
	if !hdr.Time.Equal(t100) {
		t.Errorf("Time = %v, want %v", hdr.Time, t100)
	}
	if hdr.AnchorTxHash == ([32]byte{}) {
		t.Error("AnchorTxHash should be populated from the anchor txn")
	}

	hdr2, err := src.Header(context.Background(), 101)
	if err != nil {
		t.Fatal(err)
	}
	if hdr2.StateTreeRoot != stateRoot101 {
		t.Error("Header(101) returned wrong state root")
	}
	if hdr2.AnchorTxHash == hdr.AnchorTxHash {
		t.Error("two different blocks shouldn't share AnchorTxHash")
	}
}

func TestAPISource_UnknownHeightReturnsErrNoSuchHeight(t *testing.T) {
	partition := protocol.DnUrl()
	anchorPool := partition.JoinPath(protocol.AnchorPool)
	q := &fakeAnchorQuerier{
		scope: anchorPool,
		records: []*api.ChainEntryRecord[api.Record]{
			makeAnchor(t, partition, 100, time.Now(), [32]byte{0x01}),
		},
	}
	src := NewAPISource(api.Querier2{Querier: q}, anchorPool)
	_, err := src.Header(context.Background(), 999)
	if !errors.Is(err, ErrNoSuchHeight) {
		t.Errorf("err = %v, want ErrNoSuchHeight chain", err)
	}
}

// TestAPISource_PaginatesUntilTarget ensures the source pages
// through the anchor pool when the requested height isn't on the
// first page.
func TestAPISource_PaginatesUntilTarget(t *testing.T) {
	partition := protocol.DnUrl()
	anchorPool := partition.JoinPath(protocol.AnchorPool)
	now := time.Unix(1700000000, 0).UTC()

	var records []*api.ChainEntryRecord[api.Record]
	for i := uint64(1); i <= 10; i++ {
		records = append(records, makeAnchor(t, partition, i, now, [32]byte{byte(i)}))
	}
	q := &fakeAnchorQuerier{scope: anchorPool, records: records}

	src := NewAPISource(api.Querier2{Querier: q}, anchorPool)
	src.PageSize = 3 // force multiple pages

	// Request a height on the 4th page (height 10).
	hdr, err := src.Header(context.Background(), 10)
	if err != nil {
		t.Fatalf("Header(10): %v", err)
	}
	if hdr.Height != 10 {
		t.Errorf("Height = %d, want 10", hdr.Height)
	}
}

func TestAPISource_OperatorsDeltaStubReturnsNil(t *testing.T) {
	src := NewAPISource(api.Querier2{}, protocol.DnUrl().JoinPath(protocol.AnchorPool))
	deltas, err := src.OperatorsDeltaAt(context.Background(), 1)
	if err != nil {
		t.Fatalf("OperatorsDeltaAt: %v", err)
	}
	if deltas != nil {
		t.Errorf("OperatorsDeltaAt stub should return nil, got %v", deltas)
	}
}

// Compile-time check.
var _ HeaderSource = (*APISource)(nil)

// fakeBlockQuerier extends fakeAnchorQuerier behavior to also handle
// QueryMinorBlock so OperatorsDeltaAt can be exercised. Anchors and
// minor blocks live behind the same Querier interface, dispatched on
// the query type.
type fakeBlockQuerier struct {
	anchorScope *url.URL
	anchorRecs  []*api.ChainEntryRecord[api.Record]
	partScope   *url.URL
	blocks      map[uint64]*api.MinorBlockRecord
}

func (f *fakeBlockQuerier) Query(_ context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	switch q := query.(type) {
	case *api.ChainQuery:
		if !scope.Equal(f.anchorScope) {
			return nil, errors.New("fake: unexpected anchor scope " + scope.String())
		}
		start := uint64(0)
		count := uint64(len(f.anchorRecs))
		if q.Range != nil {
			start = q.Range.Start
			if q.Range.Count != nil {
				count = *q.Range.Count
			}
		}
		end := start + count
		if end > uint64(len(f.anchorRecs)) {
			end = uint64(len(f.anchorRecs))
		}
		if start > uint64(len(f.anchorRecs)) {
			start = uint64(len(f.anchorRecs))
		}
		page := &api.RecordRange[*api.ChainEntryRecord[api.Record]]{
			Total: uint64(len(f.anchorRecs)),
			Start: start,
		}
		page.Records = append(page.Records, f.anchorRecs[start:end]...)
		return page, nil

	case *api.BlockQuery:
		if !scope.Equal(f.partScope) {
			return nil, errors.New("fake: unexpected block scope " + scope.String())
		}
		if q.Minor == nil {
			return nil, errors.New("fake: only minor-block queries supported")
		}
		block, ok := f.blocks[*q.Minor]
		if !ok {
			return nil, errors.New("fake: no such block")
		}
		return block, nil

	default:
		return nil, errors.New("fake: unsupported query type")
	}
}

// makeUpdateKeyPageEntry builds a chain entry for an UpdateKeyPage
// transaction with the given operations on the given page URL.
func makeUpdateKeyPageEntry(t *testing.T, page *url.URL, ops []protocol.KeyPageOperation) *api.ChainEntryRecord[api.Record] {
	t.Helper()
	body := &protocol.UpdateKeyPage{Operation: ops}
	txn := &protocol.Transaction{
		Header: protocol.TransactionHeader{Principal: page},
		Body:   body,
	}
	tm := &messaging.TransactionMessage{Transaction: txn}
	mr := &api.MessageRecord[messaging.Message]{ID: txn.ID(), Message: tm}
	var hashArr [32]byte
	copy(hashArr[:], txn.GetHash())
	return &api.ChainEntryRecord[api.Record]{
		Account: page,
		Name:    "main",
		Index:   0,
		Entry:   hashArr,
		Value:   mr,
	}
}

func TestAPISource_OperatorsDeltaAt_ReturnsKeyPageOps(t *testing.T) {
	partition := protocol.DnUrl()
	anchorPool := partition.JoinPath(protocol.AnchorPool)
	opPage := partition.JoinPath(protocol.Operators, "1")

	// Block 100 contains an UpdateKeyPage with one AddKeyOperation.
	addEntry := makeUpdateKeyPageEntry(t, opPage, []protocol.KeyPageOperation{
		&protocol.AddKeyOperation{Entry: protocol.KeySpecParams{KeyHash: bytes32(0x42)}},
	})
	now := time.Unix(1700000000, 0).UTC()
	block100 := &api.MinorBlockRecord{
		Index:  100,
		Time:   &now,
		Source: partition,
		Entries: &api.RecordRange[*api.ChainEntryRecord[api.Record]]{
			Total:   1,
			Records: []*api.ChainEntryRecord[api.Record]{addEntry},
		},
	}

	q := &fakeBlockQuerier{
		anchorScope: anchorPool,
		partScope:   partition,
		blocks:      map[uint64]*api.MinorBlockRecord{100: block100},
	}

	src := NewAPISource(api.Querier2{Querier: q}, anchorPool)
	src.SetOperatorsPage(opPage)

	deltas, err := src.OperatorsDeltaAt(context.Background(), 100)
	if err != nil {
		t.Fatalf("OperatorsDeltaAt: %v", err)
	}
	if len(deltas) != 1 {
		t.Fatalf("got %d deltas, want 1", len(deltas))
	}
	if deltas[0].Kind != protocol.KeyPageOperationTypeAdd.String() {
		t.Errorf("delta Kind = %q, want %q", deltas[0].Kind, protocol.KeyPageOperationTypeAdd)
	}
	if len(deltas[0].Payload) == 0 {
		t.Error("delta Payload empty")
	}
}

func TestAPISource_OperatorsDeltaAt_NoOperatorsPageYieldsNil(t *testing.T) {
	src := NewAPISource(api.Querier2{}, protocol.DnUrl().JoinPath(protocol.AnchorPool))
	deltas, err := src.OperatorsDeltaAt(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if deltas != nil {
		t.Errorf("expected nil deltas without operators page, got %v", deltas)
	}
}

func TestAPISource_OperatorsDeltaAt_FiltersNonOperatorsEntries(t *testing.T) {
	partition := protocol.DnUrl()
	anchorPool := partition.JoinPath(protocol.AnchorPool)
	opPage := partition.JoinPath(protocol.Operators, "1")
	otherPage := partition.JoinPath("network")

	// Block contains an entry on a *different* account. Should be
	// ignored — no deltas.
	now := time.Unix(1700000000, 0).UTC()
	block50 := &api.MinorBlockRecord{
		Index:  50,
		Time:   &now,
		Source: partition,
		Entries: &api.RecordRange[*api.ChainEntryRecord[api.Record]]{
			Total: 1,
			Records: []*api.ChainEntryRecord[api.Record]{
				makeUpdateKeyPageEntry(t, otherPage, []protocol.KeyPageOperation{
					&protocol.AddKeyOperation{Entry: protocol.KeySpecParams{KeyHash: bytes32(0x77)}},
				}),
			},
		},
	}

	q := &fakeBlockQuerier{
		anchorScope: anchorPool,
		partScope:   partition,
		blocks:      map[uint64]*api.MinorBlockRecord{50: block50},
	}

	src := NewAPISource(api.Querier2{Querier: q}, anchorPool)
	src.SetOperatorsPage(opPage)

	deltas, err := src.OperatorsDeltaAt(context.Background(), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(deltas) != 0 {
		t.Errorf("got %d deltas, want 0 (entry on non-operators account)", len(deltas))
	}
}

func bytes32(b byte) []byte {
	out := make([]byte, 32)
	out[0] = b
	return out
}

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

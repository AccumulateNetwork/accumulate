// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package harness

import (
	"context"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// shedFirst sheds the first query, as the node API's admission gate does
// when it is full, and answers every later one with rec.
type shedFirst struct {
	Services
	calls int
	rec   api.Record
}

func (s *shedFirst) Query(context.Context, *url.URL, api.Query) (api.Record, error) {
	s.calls++
	if s.calls == 1 {
		return nil, errors.NotReady.With("query capacity exhausted, retry later")
	}
	return s.rec, nil
}

// A condition whose status query is shed is not yet satisfied: the node said
// to retry later, and the next step asks again (#4558). In the simulator
// every node shares one gate, and a joiner's fill held it while a test
// waited on its transaction.
func TestAConditionAsksAgainWhenItsQueryIsShed(t *testing.T) {
	id := url.MustParse("alice").WithTxID([32]byte{1})
	s := &shedFirst{rec: &api.MessageRecord[messaging.Message]{
		ID:      id,
		Status:  errors.Delivered,
		Message: &messaging.TransactionMessage{Transaction: &protocol.Transaction{Body: new(protocol.SendTokens)}},
	}}
	h := New(t, s, TimeStep(0))
	h.StepUntilN(3, Txn(id).Received())
	if s.calls != 2 {
		t.Fatalf("queried %d times, want 2: the shed query and the one after it", s.calls)
	}
}

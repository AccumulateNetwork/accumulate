// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-enum  --package block enums.yml

type BlockTimerType uint64

func trackTransactionTimers(executors *map[protocol.TransactionType]chain.TransactionExecutor) (timerList []uint64) {
	//register the executor transaction timers
	for k := range *executors {
		timerList = append(timerList, k.GetEnumValue()+BlockTimerTypeTransactionOffset.GetEnumValue())
	}
	return timerList
}

type TimerRecord struct {
	timer   time.Time
	elapsed float64
	txct    int
}

type TimerSet struct {
	enable  bool
	timeRec map[uint64]*TimerRecord
	order   []uint64
}

func resolveLabel(v uint64) string {
	var ret string
	if v >= BlockTimerTypeTransactionOffset.GetEnumValue() {
		var t protocol.TransactionType
		if t.SetEnumValue(v - BlockTimerTypeTransactionOffset.GetEnumValue()) {
			ret = t.String()
		} else {
			ret = fmt.Sprintf("unknown_transaction_type_%d", v-BlockTimerTypeTransactionOffset.GetEnumValue())
		}
	} else {
		var t BlockTimerType
		if t.SetEnumValue(v) {
			ret = t.String()
		} else {
			ret = fmt.Sprintf("unknown_timer_%d", v)
		}
	}
	return ret
}

func (t *TimerSet) Store(ds *logging.DataSet) {
	for _, k := range t.order {
		v, ok := t.timeRec[k]
		if ok {
			label := resolveLabel(k)
			call_count := fmt.Sprintf("%s_call_count", label)
			ds.Save(call_count, v.txct, 10, false)
			elapsed := fmt.Sprintf("%s_elapsed", label)
			ds.Save(elapsed, v.elapsed, 10, false)
		}
	}
}

func (t *TimerSet) Initialize(executors *map[protocol.TransactionType]chain.TransactionExecutor) {
	t.enable = true
	t.timeRec = make(map[uint64]*TimerRecord)

	trackTimers := trackTransactionTimers(executors)
	trackTimers = append(trackTimers, []uint64{
		BlockTimerTypeProcessSignature.GetEnumValue(),
		BlockTimerTypeBeginBlock.GetEnumValue(),
		BlockTimerTypeCheckTx.GetEnumValue(),
		BlockTimerTypeDeliverTx.GetEnumValue(),
		BlockTimerTypeEndBlock.GetEnumValue(),
		BlockTimerTypeCommit.GetEnumValue(),
		BlockTimerTypeSigning.GetEnumValue(),
		BlockTimerTypeNetworkAccountUpdates.GetEnumValue(),
		BlockTimerTypeProcessRemoteSignatures.GetEnumValue(),
		BlockTimerTypeProcessTransaction.GetEnumValue(),
	}...)

	for _, r := range trackTimers {
		t.timeRec[r] = new(TimerRecord)
		t.order = append(t.order, r)
	}
}

func (t *TimerSet) Reset() {
	if t.enable {
		for _, r := range t.timeRec {
			r.txct = 0
			r.elapsed = 0.0
		}
	}
}

func (t *TimerSet) Start(record any) *TimerRecord {
	if t.enable {
		var key uint64
		switch v := record.(type) {
		case BlockTimerType:
			key = v.GetEnumValue()
		case protocol.TransactionType:
			key = v.GetEnumValue() + BlockTimerTypeTransactionOffset.GetEnumValue()
		default:
			return nil
		}
		r, ok := t.timeRec[key]
		if !ok {
			r = &TimerRecord{}
			t.timeRec[key] = r
		}
		r.timer = time.Now()
		return r
	}
	return nil
}

func (t *TimerSet) Stop(r *TimerRecord) {
	if r != nil {
		r.txct++
		r.elapsed += time.Since(r.timer).Seconds()
	}
}

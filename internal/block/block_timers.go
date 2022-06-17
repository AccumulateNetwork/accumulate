package block

import (
	"fmt"
	"gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"time"
)

type BlockTimerType uint64

func trackBlockTimers(executors []chain.TransactionExecutor) (timerList []uint64) {
	//register the executor timers
	for _, x := range executors {
		timerList = append(timerList, x.Type().GetEnumValue()+BlockTimerTypeTransactionOffset.GetEnumValue())
	}

	timerList = append(timerList, []uint64{
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
	for k, v := range t.timeRec {
		label := resolveLabel(k)
		call_count := fmt.Sprintf("%s_call_count", label)
		ds.Save(call_count, v.txct, len(call_count), false)
		elapsed := fmt.Sprintf("%s_elapsed", label)
		ds.Save(elapsed, v.elapsed, len(elapsed), false)
	}
}

func (t *TimerSet) Initialize(executors []chain.TransactionExecutor) {
	if t.enable {
		t.timeRec = make(map[uint64]*TimerRecord)
		for _, r := range trackBlockTimers(executors) {
			t.timeRec[r] = new(TimerRecord)
		}
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
		r.txct++
		r.timer = time.Now()
		return r
	}
	return nil
}

func (t *TimerSet) Stop(r *TimerRecord) {
	if r != nil {
		r.elapsed = time.Since(r.timer).Seconds()
	}
}

// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
)

// Following one transaction from acceptance to execution.
//
// In run 20260822T061030Z, 95 of 100 accepted transactions never executed and
// left no trace anywhere: no error, no rejection, no requeue, no re-proposal,
// no eviction (#4132). The submit path returns errors correctly, so the
// workers took them in — and then nothing. The leg from acceptance to batch
// had no logging at all, so there was no way to tell a consensus loss from an
// execution loss.
//
// This gives every transaction a stable short identity — the hash of its
// bytes, which is what the worker actually holds — and logs it at each hand-off:
//
//	accepted  (Worker.Submit)      tx -> worker
//	batched   (enqueueBatch)       tx -> batch digest
//	executed  (ExecutorBridge)     batch -> block
//
// Off by default: at load this is several lines per transaction per node, and
// the point is to diagnose, not to pay for the diagnosis forever. Set
// ACC_TX_TRACE=1 for a run that is hunting lost transactions.
var txTraceEnabled = func() bool {
	v := strings.ToLower(os.Getenv("ACC_TX_TRACE"))
	return v == "1" || v == "true" || v == "yes"
}()

// TxTraceEnabled reports whether transaction tracing is on.
func TxTraceEnabled() bool { return txTraceEnabled }

// txID is the short, stable identity used in trace lines. It is the hash of
// the raw bytes, so the same transaction is recognisable at every stage and on
// every node without parsing it.
func txID(tx []byte) string {
	h := sha256.Sum256(tx)
	return hex.EncodeToString(h[:6])
}

// txIDs maps a batch's transactions to their short ids.
func txIDs(txs [][]byte) []string {
	out := make([]string, 0, len(txs))
	for _, tx := range txs {
		out = append(out, txID(tx))
	}
	return out
}

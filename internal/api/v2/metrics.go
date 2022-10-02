// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

// Get the increase over some duration, average that by chain (by BVC), sum that
const metricTPS = "sum (avg by (chain_id) (increase(tendermint_consensus_total_txs[%v])))"

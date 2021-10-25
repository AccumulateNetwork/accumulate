package api

// Get the increase over some duration, average that by chain (by BVC), sum that
const metricTPS = "sum (avg by (chain_id) (increase(tendermint_consensus_total_txs[%v])))"

//go:build mainnet
// +build mainnet

// TODO Flip the flag to default to mainnet

package protocol

const IsTestNet = false

// InitialAcmeOracle is the oracle value at activation. Set the initial price to
// 1/5 fct price * 1/4 market cap dilution = 1/20 fct price for this exercise,
// we'll assume that 1 FCT = $1, so initial ACME price is $0.05.
const InitialAcmeOracle = 0.05

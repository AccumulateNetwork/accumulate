//go:build !mainnet && !testing
// +build !mainnet,!testing

package protocol

const IsTestNet = true
const IsRunningTests = false

// InitialAcmeOracle is the oracle value at launch. Set the oracle super high to
// make life easier on the testnet.
const InitialAcmeOracle = 5000

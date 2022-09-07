//go:build testing
// +build testing

package protocol

var IsTestNet = true

const IsRunningTests = true

// InitialAcmeOracle is the oracle value at launch. Set the oracle super high to
// make life easier on the testnet.
const InitialAcmeOracle = 5000

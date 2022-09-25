//go:build !mainnet
// +build !mainnet

package protocol

const IsTestNet = true

// InitialAcmeOracle is the oracle value at launch. Set the oracle super high to
// make life easier on the testnet.
const InitialAcmeOracle = 5000

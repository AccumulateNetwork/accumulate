// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !mainnet
// +build !mainnet

package protocol

const IsTestNet = true

// InitialAcmeOracle is the oracle value at launch. Set the oracle super high to
// make life easier on the testnet.
const InitialAcmeOracle = 5000

// MIT License
//
// Copyright 2018 Canonical Ledgers, LLC
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
// IN THE SOFTWARE.

package factom

import (
	"fmt"
	"strings"
)

var (
	mainnetID  = [...]byte{0xFA, 0x92, 0xE5, 0xA2}
	testnetID  = [...]byte{0x88, 0x3e, 0x09, 0x3b}
	localnetID = [...]byte{0xFA, 0x92, 0xE5, 0xA4}
)

// MainnetID returns the Mainnet NetworkID, 0xFA92E5A2.
func MainnetID() NetworkID { return mainnetID }

// TestnetID returns the Testnet NetworkID, 0x883e093b.
func TestnetID() NetworkID { return testnetID }

// LocalnetID returns the Localnet NetworkID, 0xFA92E5A4.
func LocalnetID() NetworkID { return localnetID }

// NetworkID represents the 4 byte magic number that helps identify distinct
// Factom networks.
//
// NetworkID conforms to the flag.Value interface.
type NetworkID [4]byte

// String returns "mainnet", "testnet", "localnet", or "custom: 0x...".
func (n NetworkID) String() string {
	switch n {
	case mainnetID:
		return "mainnet"
	case testnetID:
		return "testnet"
	case localnetID:
		return "localnet"
	default:
		return "custom: 0x" + Bytes(n[:]).String()
	}
}

// Set sets n to the NetworkID corresponding to netIDStr, which can be "main",
// "mainnet", "test", "testnet", "local", "localnet" or a 4 byte hex encoded
// string for a custom NetworkID.
func (n *NetworkID) Set(netIDStr string) error {
	switch strings.ToLower(netIDStr) {
	case "main", "mainnet":
		*n = mainnetID
	case "test", "testnet":
		*n = testnetID
	case "local", "localnet":
		*n = localnetID
	default:
		if netIDStr[:2] == "0x" {
			// omit leading 0x
			netIDStr = netIDStr[2:]
		}
		var b Bytes
		if err := b.Set(netIDStr); err != nil {
			return err
		}
		if len(b) != len(n[:]) {
			return fmt.Errorf("invalid length")
		}
		copy(n[:], b)
	}
	return nil
}

// IsMainnet returns true if n is the Mainnet NetworkID.
func (n NetworkID) IsMainnet() bool {
	return n == mainnetID
}

// IsTestnet returns true if n is the Testnet NetworkID.
func (n NetworkID) IsTestnet() bool {
	return n == testnetID
}

// IsLocalnet returns true if n is the Localnet NetworkID.
func (n NetworkID) IsLocalnet() bool {
	return n == localnetID
}

// IsCustom returns true if n is not the Mainnet, Testnet, or Localnet
// NetworkID.
func (n NetworkID) IsCustom() bool {
	return !n.IsMainnet() && !n.IsTestnet() && !n.IsLocalnet()
}

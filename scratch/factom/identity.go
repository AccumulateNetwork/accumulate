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
	"context"
	"fmt"
)

// ValidIdentityChainID returns true if the chainID matches the pattern for an
// Identity Chain ID.
//
// The Identity Chain specification can be found here:
// https://github.com/FactomProject/FactomDocs/blob/master/Identity.md#factom-identity-chain-creation
func ValidIdentityChainID(chainID Bytes) bool {
	if len(chainID) == len(Bytes32{}) &&
		chainID[0] == 0x88 &&
		chainID[1] == 0x88 &&
		chainID[2] == 0x88 {
		return true
	}
	return false
}

// ValidIdentityNameIDs returns true if the nameIDs match the pattern for a
// valid Identity Chain. The nameIDs for a chain are the ExtIDs of the first
// entry in the chain.
//
// The Identity Chain specification can be found here:
// https://github.com/FactomProject/FactomDocs/blob/master/Identity.md#factom-identity-chain-creation
func ValidIdentityNameIDs(nameIDs []Bytes) bool {
	if len(nameIDs) == 7 &&
		len(nameIDs[0]) == 1 && nameIDs[0][0] == 0x00 &&
		string(nameIDs[1]) == "Identity Chain" &&
		len(nameIDs[2]) == len(ID1Key{}) &&
		len(nameIDs[3]) == len(ID2Key{}) &&
		len(nameIDs[4]) == len(ID3Key{}) &&
		len(nameIDs[5]) == len(ID4Key{}) {
		return true
	}
	return false
}

// Identity represents the Token Issuer's Identity Chain and the public ID1Key.
type Identity struct {
	ID1Key *ID1Key
	Height uint32
	Entry
}

// IsPopulated returns true if the Identity has been populated with an ID1Key.
func (i Identity) IsPopulated() bool {
	return i.ID1Key != nil && !(*Bytes32)(i.ID1Key).IsZero()
}

// Get validates i.ChainID as an Identity Chain and parses out the ID1Key.
func (i *Identity) Get(ctx context.Context, c *Client) error {
	if i.ChainID == nil {
		return fmt.Errorf("ChainID is nil")
	}
	if i.IsPopulated() {
		return nil
	}
	if !ValidIdentityChainID(i.ChainID[:]) {
		return nil
	}

	// Get first entry block of Identity Chain.
	eb := EBlock{ChainID: i.ChainID}
	if err := eb.GetFirst(ctx, c); err != nil {
		return err
	}

	// Get first entry of first entry block.
	first := eb.Entries[0]
	if err := first.Get(ctx, c); err != nil {
		return err
	}

	if !ValidIdentityNameIDs(first.ExtIDs) {
		return nil
	}

	i.Height = eb.Height
	i.Entry = first
	i.ID1Key = new(ID1Key)
	copy(i.ID1Key[:], first.ExtIDs[2])

	return nil
}

// UnmarshalBinary calls i.Entry.UnmarshalBinary and then performs additional
// validation checks on the ChainID and NameID formats.
func (i *Identity) UnmarshalBinary(data []byte) error {
	if err := i.Entry.UnmarshalBinary(data); err != nil {
		return err
	}
	if !ValidIdentityChainID(i.ChainID[:]) {
		return fmt.Errorf("invalid identity ChainID format")
	}
	if !ValidIdentityNameIDs(i.ExtIDs) {
		return fmt.Errorf("invalid identity NameID format")
	}
	if *i.ChainID != ComputeChainID(i.ExtIDs) {
		return fmt.Errorf("invalid ExtIDs: Chain ID mismatch")
	}
	i.ID1Key = new(ID1Key)
	copy(i.ID1Key[:], i.ExtIDs[2])
	return nil
}

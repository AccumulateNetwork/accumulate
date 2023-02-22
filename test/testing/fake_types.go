// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testing

import (
	"bytes"
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (*FakeTransactionBody) UnmarshalFieldsFrom(*encoding.Reader) error { panic("not supported") }
func (*FakeSignature) UnmarshalFieldsFrom(*encoding.Reader) error       { panic("not supported") }
func (*FakeLiteAccount) UnmarshalFieldsFrom(*encoding.Reader) error     { panic("not supported") }
func (*FakeAccount) UnmarshalFieldsFrom(*encoding.Reader) error         { panic("not supported") }
func (*FakeSigner) UnmarshalFieldsFrom(*encoding.Reader) error          { panic("not supported") }

var _ protocol.TransactionBody = (*FakeTransactionBody)(nil)

func (f *FakeTransactionBody) Type() protocol.TransactionType { return f.TheType }

var _ protocol.Signature = (*FakeSignature)(nil)
var _ protocol.KeySignature = (*FakeSignature)(nil)

func (f *FakeSignature) Type() protocol.SignatureType       { return f.TheType }
func (f *FakeSignature) GetVote() protocol.VoteType         { return f.Vote }
func (f *FakeSignature) Verify(sigMdHash, hash []byte) bool { return true }
func (f *FakeSignature) Hash() []byte                       { return make([]byte, 32) }
func (f *FakeSignature) Metadata() protocol.Signature       { return f }
func (f *FakeSignature) Initiator() (hash.Hasher, error)    { return nil, nil }
func (f *FakeSignature) GetSigner() *url.URL                { return f.Signer }
func (f *FakeSignature) RoutingLocation() *url.URL          { return f.Signer }
func (f *FakeSignature) GetSignerVersion() uint64           { return f.SignerVersion }
func (f *FakeSignature) GetTimestamp() uint64               { return f.Timestamp }
func (f *FakeSignature) GetPublicKey() []byte               { return f.PublicKey }
func (f *FakeSignature) GetSignature() []byte               { return make([]byte, 32) }
func (f *FakeSignature) GetTransactionHash() [32]byte       { return [32]byte{} }

func (f *FakeSignature) GetPublicKeyHash() []byte {
	if f.Type() == protocol.SignatureTypeRCD1 {
		return protocol.GetRCDHashFromPublicKey(f.PublicKey, 1)
	}
	hash := sha256.Sum256(f.PublicKey)
	return hash[:]
}

var _ protocol.Account = (*FakeLiteAccount)(nil)

func (f *FakeLiteAccount) Type() protocol.AccountType { return f.TheType }
func (f *FakeLiteAccount) GetUrl() *url.URL           { return f.Url }
func (f *FakeLiteAccount) StripUrl()                  { f.Url = f.GetUrl().StripExtras() }

var _ protocol.FullAccount = (*FakeAccount)(nil)

func (f *FakeAccount) GetAuth() *protocol.AccountAuth { return &f.AccountAuth }

var _ protocol.Signer = (*FakeSigner)(nil)

func (f *FakeSigner) GetAuthority() *url.URL             { return f.Url }
func (f *FakeSigner) GetVersion() uint64                 { return f.Version }
func (f *FakeSigner) CreditCredits(amount uint64)        { f.CreditBalance = amount }
func (f *FakeSigner) GetCreditBalance() uint64           { return f.CreditBalance }
func (f *FakeSigner) CanDebitCredits(amount uint64) bool { return amount <= f.CreditBalance }

func (f *FakeSigner) GetSignatureThreshold() uint64 {
	if f.Threshold == 0 {
		return 1
	}
	return f.Threshold
}

func (f *FakeSigner) EntryByKey(key []byte) (int, protocol.KeyEntry, bool) {
	keyHash := sha256.Sum256(key)
	return f.EntryByKeyHash(keyHash[:])
}

func (f *FakeSigner) EntryByKeyHash(keyHash []byte) (int, protocol.KeyEntry, bool) {
	for i, entry := range f.Keys {
		if bytes.Equal(entry.PublicKeyHash, keyHash) {
			return i, entry, true
		}
	}

	return -1, nil, false
}

func (f *FakeSigner) EntryByDelegate(owner *url.URL) (int, protocol.KeyEntry, bool) {
	for i, entry := range f.Keys {
		if owner.Equal(entry.Delegate) {
			return i, entry, true
		}
	}

	return -1, nil, false
}

func (f *FakeSigner) DebitCredits(amount uint64) bool {
	if !f.CanDebitCredits(amount) {
		return false
	}

	f.CreditBalance -= amount
	return true
}

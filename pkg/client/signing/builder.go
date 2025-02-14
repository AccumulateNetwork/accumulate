// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package signing

import (
	"fmt"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type InitHashMode int

const (
	// Initiate with a Merkle hash.
	//
	// Deprecated: Use [InitWithSimpleHash].
	InitWithMerkleHash InitHashMode = iota

	// Initiate with a simple hash.
	InitWithSimpleHash
)

type Builder struct {
	InitMode   InitHashMode
	Type       protocol.SignatureType
	Url        *url.URL
	Delegators []*url.URL
	Signer     Signer
	Version    uint64
	Vote       protocol.VoteType
	Timestamp  Timestamp
	Memo       string
	Data       []byte

	// Ignore64Byte (when set) stops the signature builder from automatically
	// correcting a transaction header or body that marshals to 64 bytes.
	Ignore64Byte bool
}

func (s *Builder) Import(sig protocol.Signature) (*Builder, error) {
	s.Type = sig.Type()

	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		s.Url = sig.Signer
		s.Version = sig.SignerVersion
		s.Timestamp = TimestampFromValue(sig.Timestamp)
	case *protocol.ED25519Signature:
		s.Url = sig.Signer
		s.Version = sig.SignerVersion
		s.Timestamp = TimestampFromValue(sig.Timestamp)
		s.Memo = sig.Memo
		s.Data = sig.Data
	case *protocol.RCD1Signature:
		s.Url = sig.Signer
		s.Version = sig.SignerVersion
		s.Timestamp = TimestampFromValue(sig.Timestamp)
		s.Memo = sig.Memo
		s.Data = sig.Data
	case *protocol.BTCSignature:
		s.Url = sig.Signer
		s.Version = sig.SignerVersion
		s.Timestamp = TimestampFromValue(sig.Timestamp)
		s.Memo = sig.Memo
		s.Data = sig.Data
	case *protocol.BTCLegacySignature:
		s.Url = sig.Signer
		s.Version = sig.SignerVersion
		s.Timestamp = TimestampFromValue(sig.Timestamp)
		s.Memo = sig.Memo
		s.Data = sig.Data
	case *protocol.ETHSignature:
		s.Url = sig.Signer
		s.Version = sig.SignerVersion
		s.Timestamp = TimestampFromValue(sig.Timestamp)
		s.Memo = sig.Memo
		s.Data = sig.Data
	case *protocol.RsaSha256Signature:
		s.Url = sig.Signer
		s.Version = sig.SignerVersion
		s.Timestamp = TimestampFromValue(sig.Timestamp)
		s.Memo = sig.Memo
		s.Data = sig.Data
	case *protocol.EcdsaSha256Signature:
		s.Url = sig.Signer
		s.Version = sig.SignerVersion
		s.Timestamp = TimestampFromValue(sig.Timestamp)
		s.Memo = sig.Memo
		s.Data = sig.Data
	case *protocol.DelegatedSignature:
		_, err := s.Import(sig.Signature)
		if err != nil {
			return nil, err
		}
		s.Delegators = append(s.Delegators, sig.Delegator)
	default:
		return nil, fmt.Errorf("unsupported signature type %v", sig.Type())
	}

	return s, nil
}

func (s *Builder) Copy() *Builder {
	t := *s
	t.Delegators = make([]*url.URL, len(s.Delegators))
	copy(t.Delegators, s.Delegators)
	return &t
}

// UseSimpleHash initiate with a simple hash.
func (s *Builder) UseSimpleHash() *Builder {
	s.InitMode = InitWithSimpleHash
	return s
}

// UseMerkleHash initiate with a Merkle hash.
//
// Deprecated: Use [Builder.UseSimpleHash].
func (s *Builder) UseMerkleHash() *Builder {
	s.InitMode = InitWithMerkleHash
	return s
}

func (s *Builder) SetType(typ protocol.SignatureType) *Builder {
	s.Type = typ
	return s
}

func (s *Builder) SetUrl(u *url.URL) *Builder {
	s.Url = u
	return s
}

func (s *Builder) SetKeyPageUrl(bookUrl *url.URL, pageIndex uint64) *Builder {
	s.Url = protocol.FormatKeyPageUrl(bookUrl, pageIndex)
	return s
}

func (s *Builder) SetPrivateKey(privKey []byte) *Builder {
	s.Signer = PrivateKey(privKey)
	return s
}

func (s *Builder) AddDelegator(delegator *url.URL) *Builder {
	s.Delegators = append(s.Delegators, delegator)
	return s
}

func (s *Builder) SetSigner(signer Signer) *Builder {
	s.Signer = signer
	return s
}

func (s *Builder) SetVersion(version uint64) *Builder {
	s.Version = version
	return s
}

func (s *Builder) ClearTimestamp() *Builder {
	s.Timestamp = nil
	return s
}

func (s *Builder) SetTimestamp(timestamp uint64) *Builder {
	s.Timestamp = TimestampFromValue(timestamp)
	return s
}

func (s *Builder) SetTimestampWithVar(timestamp *uint64) *Builder {
	s.Timestamp = (*TimestampFromVariable)(timestamp)
	return s
}

func (s *Builder) SetTimestampToNow() *Builder {
	s.Timestamp = TimestampFromValue(time.Now().UTC().UnixMilli())
	return s
}

func (s *Builder) UseFaucet() *Builder {
	f := protocol.Faucet.Signer()
	s.Signer = f
	s.Url = protocol.FaucetUrl.RootIdentity()
	s.Timestamp = TimestampFromValue(f.Timestamp())
	s.Version = f.Version()
	return s
}

func (s *Builder) prepare(init bool) (protocol.KeySignature, error) {
	var errs []string
	if s.Url == nil {
		errs = append(errs, "missing signer URL")
	}
	if s.Signer == nil {
		errs = append(errs, "missing signer")
	}
	if init && s.Version == 0 {
		errs = append(errs, "missing version")
	}
	if init && s.Timestamp == nil {
		errs = append(errs, "missing timestamp")
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("cannot prepare signature: %s", strings.Join(errs, ", "))
	}

	switch s.Type {
	case protocol.SignatureTypeUnknown:
		s.Type = protocol.SignatureTypeED25519

	case protocol.SignatureTypeLegacyED25519,
		protocol.SignatureTypeED25519,
		protocol.SignatureTypeRCD1,
		protocol.SignatureTypeBTC,
		protocol.SignatureTypeETH,
		protocol.SignatureTypeRsaSha256,
		protocol.SignatureTypeEcdsaSha256,
		protocol.SignatureTypeTypedData,
		protocol.SignatureTypeBTCLegacy:

	case protocol.SignatureTypeReceipt, protocol.SignatureTypePartition:
		// Calling Sign for SignatureTypeReceipt or SignatureTypeSynthetic makes zero sense
		panic(fmt.Errorf("invalid attempt to generate signature of type %v", s.Type))

	default:
		return nil, fmt.Errorf("unknown signature type %v", s.Type)
	}

	var timestamp uint64
	var err error
	if s.Timestamp != nil {
		timestamp, err = s.Timestamp.Get()
		if err != nil {
			return nil, err
		}
	}

	switch s.Type {
	case protocol.SignatureTypeLegacyED25519:
		sig := new(protocol.LegacyED25519Signature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = timestamp
		sig.Vote = s.Vote
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeUnknown, protocol.SignatureTypeED25519:
		sig := new(protocol.ED25519Signature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = timestamp
		sig.Vote = s.Vote
		sig.Memo = s.Memo
		sig.Data = s.Data
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeRCD1:
		sig := new(protocol.RCD1Signature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = timestamp
		sig.Vote = s.Vote
		sig.Memo = s.Memo
		sig.Data = s.Data
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeBTC:
		sig := new(protocol.BTCSignature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = timestamp
		sig.Vote = s.Vote
		sig.Memo = s.Memo
		sig.Data = s.Data
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeBTCLegacy:
		sig := new(protocol.BTCLegacySignature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = timestamp
		sig.Vote = s.Vote
		sig.Memo = s.Memo
		sig.Data = s.Data
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeETH:
		sig := new(protocol.ETHSignature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = timestamp
		sig.Vote = s.Vote
		sig.Memo = s.Memo
		sig.Data = s.Data
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeRsaSha256:
		sig := new(protocol.RsaSha256Signature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = timestamp
		sig.Vote = s.Vote
		sig.Memo = s.Memo
		sig.Data = s.Data
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeEcdsaSha256:
		sig := new(protocol.EcdsaSha256Signature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = timestamp
		sig.Vote = s.Vote
		sig.Memo = s.Memo
		sig.Data = s.Data
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeTypedData:
		sig := new(protocol.TypedDataSignature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = timestamp
		sig.Vote = s.Vote
		sig.Memo = s.Memo
		sig.Data = s.Data
		return sig, s.Signer.SetPublicKey(sig)
	default:
		panic("unreachable")
	}
}

func (s *Builder) sign(sig protocol.Signature, sigMdHash, hash []byte) error {
	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		sig.TransactionHash = *(*[32]byte)(hash)
	case *protocol.ED25519Signature:
		sig.TransactionHash = *(*[32]byte)(hash)
	case *protocol.RCD1Signature:
		sig.TransactionHash = *(*[32]byte)(hash)
	case *protocol.BTCSignature:
		sig.TransactionHash = *(*[32]byte)(hash)
	case *protocol.BTCLegacySignature:
		sig.TransactionHash = *(*[32]byte)(hash)
	case *protocol.ETHSignature:
		sig.TransactionHash = *(*[32]byte)(hash)
	case *protocol.RsaSha256Signature:
		sig.TransactionHash = *(*[32]byte)(hash)
	case *protocol.EcdsaSha256Signature:
		sig.TransactionHash = *(*[32]byte)(hash)
	case *protocol.DelegatedSignature:
		if sigMdHash == nil {
			sigMdHash = sig.Metadata().Hash()
		}
		return s.sign(sig.Signature, sigMdHash, hash)
	default:
		panic("unreachable")
	}

	return s.Signer.Sign(sig, sigMdHash, hash)
}

func (s *Builder) Sign(message []byte) (protocol.Signature, error) {
	var sig protocol.Signature
	sig, err := s.prepare(false)
	if err != nil {
		return nil, err
	}

	for _, delegator := range s.Delegators {
		sig = &protocol.DelegatedSignature{
			Delegator: delegator,
			Signature: sig,
		}
	}

	return sig, s.sign(sig, nil, message)
}

func (s *Builder) Initiate(txn *protocol.Transaction) (protocol.Signature, error) {
	var sig protocol.Signature
	sig, err := s.prepare(true)
	if err != nil {
		return nil, err
	}

	for _, delegator := range s.Delegators {
		sig = &protocol.DelegatedSignature{
			Delegator: delegator,
			Signature: sig,
		}
	}

	if s.InitMode == InitWithSimpleHash {
		txn.Header.Initiator = *(*[32]byte)(sig.Metadata().Hash())
	} else {
		init, err := sig.(protocol.UserSignature).Initiator()
		if err != nil {
			return nil, err
		}

		txn.Header.Initiator = *(*[32]byte)(init.MerkleHash())
	}

	// // Adjust the header length
	// txn, err = s.adjustHeader(txn)
	// if err != nil {
	// 	return nil, err
	// }

	return sig, s.sign(sig, nil, txn.GetHash())
}

func (s *Builder) InitiateSynthetic(txn *protocol.Transaction, dest *url.URL) (*protocol.PartitionSignature, error) {
	var errs []string
	if s.Url == nil {
		errs = append(errs, "missing signer")
	}
	if s.Version == 0 {
		errs = append(errs, "missing sequence number")
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("cannot prepare signature: %s", strings.Join(errs, ", "))
	}

	initSig := new(protocol.PartitionSignature)
	initSig.SourceNetwork = s.Url
	initSig.DestinationNetwork = dest
	initSig.SequenceNumber = s.Version

	// Ignore InitMode, always use a simple hash
	txn.Header.Initiator = *(*[32]byte)(initSig.Metadata().Hash())

	initSig.TransactionHash = *(*[32]byte)(txn.GetHash())
	return initSig, nil
}

// func (b *Builder) adjustHeader(txn *protocol.Transaction) (*protocol.Transaction, error) {
// 	if b.Ignore64Byte {
// 		return txn, nil
// 	}

// 	// Is the header exactly 64 bytes?
// 	header, err := txn.Header.MarshalBinary()
// 	if err != nil {
// 		return nil, errors.EncodingError.WithFormat("marshal header: %w", err)
// 	}
// 	if len(header) != 64 {
// 		return txn, nil
// 	}

// 	header = append(header, 0)
// 	txn.Header = protocol.TransactionHeader{}
// 	err = txn.Header.UnmarshalBinary(header)
// 	if err != nil {
// 		return nil, errors.EncodingError.WithFormat("unmarshal header: %w", err)
// 	}

// 	// Copy to reset the cached hash if there is one
// 	return txn.Copy(), nil
// }

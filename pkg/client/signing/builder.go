// Copyright 2023 The Accumulate Authors
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
	InitWithMerkleHash InitHashMode = iota
	InitWithSimpleHash
)

type Builder struct {
	InitMode   InitHashMode
	Type       protocol.SignatureType
	Url        *url.URL
	Delegators []*url.URL
	Signer     Signer
	Version    uint64
	Timestamp  Timestamp
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
	case *protocol.RCD1Signature:
		s.Url = sig.Signer
		s.Version = sig.SignerVersion
		s.Timestamp = TimestampFromValue(sig.Timestamp)
	case *protocol.BTCSignature:
		s.Url = sig.Signer
		s.Version = sig.SignerVersion
		s.Timestamp = TimestampFromValue(sig.Timestamp)
	case *protocol.BTCLegacySignature:
		s.Url = sig.Signer
		s.Version = sig.SignerVersion
		s.Timestamp = TimestampFromValue(sig.Timestamp)
	case *protocol.ETHSignature:
		s.Url = sig.Signer
		s.Version = sig.SignerVersion
		s.Timestamp = TimestampFromValue(sig.Timestamp)
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

func (s *Builder) UseSimpleHash() *Builder {
	s.InitMode = InitWithSimpleHash
	return s
}

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
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeUnknown, protocol.SignatureTypeED25519:
		sig := new(protocol.ED25519Signature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = timestamp
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeRCD1:
		sig := new(protocol.RCD1Signature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = timestamp
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeBTC:
		sig := new(protocol.BTCSignature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = timestamp
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeBTCLegacy:
		sig := new(protocol.BTCLegacySignature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = timestamp
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeETH:
		sig := new(protocol.ETHSignature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = timestamp
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
		init, err := sig.(protocol.InitiatorSignature).Initiator()
		if err != nil {
			return nil, err
		}

		txn.Header.Initiator = *(*[32]byte)(init.MerkleHash())
	}

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

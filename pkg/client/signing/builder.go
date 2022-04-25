package signing

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type InitHashMode int

const (
	InitWithMerkleHash InitHashMode = iota
	InitWithSimpleHash
)

type Builder struct {
	InitMode  InitHashMode
	Type      protocol.SignatureType
	Url       *url.URL
	Signer    Signer
	Version   uint64
	Timestamp uint64
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

func (s *Builder) SetSigner(signer Signer) *Builder {
	s.Signer = signer
	return s
}

func (s *Builder) SetVersion(version uint64) *Builder {
	s.Version = version
	return s
}

func (s *Builder) SetTimestamp(timestamp uint64) *Builder {
	s.Timestamp = timestamp
	return s
}

func (s *Builder) SetTimestampWithVar(timestamp *uint64) *Builder {
	s.Timestamp = atomic.AddUint64(timestamp, 1)
	return s
}

func (s *Builder) SetTimestampToNow() *Builder {
	s.Timestamp = uint64(time.Now().UTC().UnixMilli())
	return s
}

func (s *Builder) UseFaucet() *Builder {
	f := protocol.Faucet.Signer()
	s.Signer = f
	s.Url = protocol.FaucetUrl.RootIdentity()
	s.Timestamp = f.Timestamp()
	s.Version = f.Version()
	return s
}

func (s *Builder) prepare(init bool) (protocol.Signature, error) {
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
	if init && s.Timestamp == 0 {
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

	case protocol.SignatureTypeReceipt, protocol.SignatureTypeSynthetic, protocol.SignatureTypeInternal:
		// Calling Sign for SignatureTypeReceipt or SignatureTypeSynthetic makes zero sense
		panic(fmt.Errorf("invalid attempt to generate signature of type %v!", s.Type))

	default:
		return nil, fmt.Errorf("unknown signature type %v", s.Type)
	}

	switch s.Type {
	case protocol.SignatureTypeLegacyED25519:
		sig := new(protocol.LegacyED25519Signature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = s.Timestamp
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeUnknown, protocol.SignatureTypeED25519:
		sig := new(protocol.ED25519Signature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = s.Timestamp
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeRCD1:
		sig := new(protocol.RCD1Signature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = s.Timestamp
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeBTC:
		sig := new(protocol.BTCSignature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = s.Timestamp
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeBTCLegacy:
		sig := new(protocol.BTCLegacySignature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = s.Timestamp
		return sig, s.Signer.SetPublicKey(sig)

	case protocol.SignatureTypeETH:
		sig := new(protocol.ETHSignature)
		sig.Signer = s.Url
		sig.SignerVersion = s.Version
		sig.Timestamp = s.Timestamp
		return sig, s.Signer.SetPublicKey(sig)

	default:
		panic("unreachable")
	}
}

func (s *Builder) sign(sig protocol.Signature, hash []byte) error {
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
	default:
		panic("unreachable")
	}

	return s.Signer.Sign(sig, hash)
}

func (s *Builder) Sign(message []byte) (protocol.Signature, error) {
	sig, err := s.prepare(false)
	if err != nil {
		return nil, err
	}

	return sig, s.sign(sig, message)
}

func (s *Builder) Initiate(txn *protocol.Transaction) (protocol.Signature, error) {
	sig, err := s.prepare(true)
	if err != nil {
		return nil, err
	}

	if s.InitMode == InitWithSimpleHash {
		txn.Header.Initiator = *(*[32]byte)(sig.MetadataHash())
	} else {
		init, err := sig.InitiatorHash()
		if err != nil {
			return nil, err
		}

		txn.Header.Initiator = *(*[32]byte)(init)
	}

	return sig, s.sign(sig, txn.GetHash())
}

func (s *Builder) InitiateSynthetic(txn *protocol.Transaction, router routing.Router) (protocol.Signature, error) {
	var errs []string
	if s.Url == nil {
		errs = append(errs, "missing signer")
	}
	if s.Version == 0 {
		errs = append(errs, "missing version")
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("cannot prepare signature: %s", strings.Join(errs, ", "))
	}

	destSubnet, err := router.RouteAccount(txn.Header.Principal)
	if err != nil {
		return nil, fmt.Errorf("routing %v: %v", txn.Header.Principal, err)
	}

	initSig := new(protocol.SyntheticSignature)
	initSig.SourceNetwork = s.Url
	initSig.DestinationNetwork = protocol.SubnetUrl(destSubnet)
	initSig.SequenceNumber = s.Version

	if s.InitMode == InitWithSimpleHash {
		txn.Header.Initiator = *(*[32]byte)(initSig.MetadataHash())
	} else {
		initHash, err := initSig.InitiatorHash()
		if err != nil {
			// This should never happen
			panic(fmt.Errorf("failed to calculate the synthetic signature initiator hash: %v", err))
		}

		txn.Header.Initiator = *(*[32]byte)(initHash)
	}

	initSig.TransactionHash = *(*[32]byte)(txn.GetHash())
	return initSig, nil
}

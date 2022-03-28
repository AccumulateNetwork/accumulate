package signing

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
)

type Signer struct {
	Type       protocol.SignatureType
	Url        *url.URL
	PrivateKey []byte
	Height     uint64
	Timestamp  uint64
}

func (s *Signer) SetType(typ protocol.SignatureType) *Signer {
	s.Type = typ
	return s
}

func (s *Signer) SetUrl(u *url.URL) *Signer {
	s.Url = u
	return s
}

func (s *Signer) SetKeyPageUrl(bookUrl *url.URL, pageIndex uint64) *Signer {
	s.Url = protocol.FormatKeyPageUrl(bookUrl, pageIndex)
	return s
}

func (s *Signer) SetPrivateKey(privKey []byte) *Signer {
	s.PrivateKey = privKey
	return s
}

func (s *Signer) SetHeight(height uint64) *Signer {
	s.Height = height
	return s
}

func (s *Signer) SetTimestamp(timestamp uint64) *Signer {
	s.Timestamp = timestamp
	return s
}

func (s *Signer) SetTimestampWithVar(timestamp *uint64) *Signer {
	s.Timestamp = atomic.AddUint64(timestamp, 1)
	return s
}

func (s *Signer) SetTimestampToNow() *Signer {
	s.Timestamp = uint64(time.Now().UTC().UnixMilli())
	return s
}

func (s *Signer) prepare(init bool) (protocol.Signature, error) {
	var errs []string
	if s.Url == nil {
		errs = append(errs, "missing signer")
	}
	if len(s.PrivateKey) == 0 {
		errs = append(errs, "missing private key")
	}
	if init && s.Height == 0 {
		errs = append(errs, "missing height")
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
		fallthrough
	case protocol.SignatureTypeLegacyED25519,
		protocol.SignatureTypeED25519,
		protocol.SignatureTypeRCD1:
		if len(s.PrivateKey) != ed25519.PrivateKeySize {
			return nil, errors.New("cannot prepare signature: invalid private key")
		}

	case protocol.SignatureTypeReceipt, protocol.SignatureTypeSynthetic, protocol.SignatureTypeInternal:
		// Calling Sign for SignatureTypeReceipt or SignatureTypeSynthetic makes zero sense
		panic(fmt.Errorf("invalid attempt to generate signature of type %v!", s.Type))

	default:
		return nil, fmt.Errorf("unknown signature type %v", s.Type)
	}

	switch s.Type {
	case protocol.SignatureTypeLegacyED25519:
		sig := new(protocol.LegacyED25519Signature)
		sig.PublicKey = s.PrivateKey[32:]
		sig.Signer = s.Url
		sig.SignerVersion = s.Height
		sig.Timestamp = s.Timestamp
		return sig, nil

	case protocol.SignatureTypeUnknown, protocol.SignatureTypeED25519:
		sig := new(protocol.ED25519Signature)
		sig.PublicKey = s.PrivateKey[32:]
		sig.Signer = s.Url
		sig.SignerVersion = s.Height
		sig.Timestamp = s.Timestamp
		return sig, nil

	case protocol.SignatureTypeRCD1:
		sig := new(protocol.RCD1Signature)
		sig.PublicKey = s.PrivateKey[32:]
		sig.Signer = s.Url
		sig.SignerVersion = s.Height
		sig.Timestamp = s.Timestamp
		return sig, nil

	default:
		panic("unreachable")
	}
}

func (s *Signer) sign(sig protocol.Signature, message []byte) {
	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		withNonce := append(common.Uint64Bytes(s.Timestamp), message...)
		sig.Signature = ed25519.Sign(s.PrivateKey, withNonce)

	case *protocol.ED25519Signature:
		sig.Signature = ed25519.Sign(s.PrivateKey, message)

	case *protocol.RCD1Signature:
		sig.Signature = ed25519.Sign(s.PrivateKey, message)

	default:
		panic("unreachable")
	}
}

func (s *Signer) Sign(message []byte) (protocol.Signature, error) {
	sig, err := s.prepare(false)
	if err != nil {
		return nil, err
	}

	s.sign(sig, message)
	return sig, nil
}

func (s *Signer) Initiate(txn *protocol.Transaction) (protocol.Signature, error) {
	sig, err := s.prepare(true)
	if err != nil {
		return nil, err
	}

	init, err := sig.InitiatorHash()
	if err != nil {
		return nil, err
	}

	txn.Header.Initiator = *(*[32]byte)(init)
	s.sign(sig, txn.GetHash())
	return sig, nil
}

func (s *Signer) InitiateSynthetic(txn *protocol.Transaction, router routing.Router) (protocol.Signature, error) {
	var errs []string
	if s.Url == nil {
		errs = append(errs, "missing signer")
	}
	if s.Height == 0 {
		errs = append(errs, "missing timestamp")
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
	initSig.SequenceNumber = s.Height

	initHash, err := initSig.InitiatorHash()
	if err != nil {
		// This should never happen
		panic(fmt.Errorf("failed to calculate the synthetic signature initiator hash: %v", err))
	}

	txn.Header.Initiator = *(*[32]byte)(initHash)
	return initSig, nil
}

func Faucet(txn *protocol.Transaction) (protocol.Signature, error) {
	fs := protocol.Faucet.Signer()
	sig := new(protocol.LegacyED25519Signature)
	sig.Signer = protocol.FaucetUrl
	sig.SignerVersion = 1
	sig.Timestamp = fs.Timestamp()
	sig.PublicKey = fs.PublicKey()

	init, err := sig.InitiatorHash()
	if err != nil {
		return nil, err
	}

	txn.Header.Initiator = *(*[32]byte)(init)
	sig.Signature = fs.Sign(txn.GetHash())
	return sig, nil
}

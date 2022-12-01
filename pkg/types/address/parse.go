package address

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/mr-tron/base58"
	"github.com/multiformats/go-multibase"
	"github.com/multiformats/go-multihash"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func Parse(s string) (Address, error) {
	if len(s) < 2 {
		return nil, errors.BadRequest.With("invalid address: too short")
	}

	if strings.HasPrefix(s, "acc://") {
		return parseLite(s)
	}

	switch s[:2] {
	case "AC": // Accumulate public
		b, err := parse2(s, sha256.Size, "AC1")
		if err != nil {
			return nil, errors.BadRequest.WithFormat("invalid AC1 address: %w", err)
		}
		return &PublicKeyHash{Type: protocol.SignatureTypeED25519, Hash: b}, nil

	case "AS": // Accumulate private
		b, err := parse2(s, ed25519.SeedSize, "AS1")
		if err != nil {
			return nil, errors.BadRequest.WithFormat("invalid AS1 address: %w", err)
		}
		key := ed25519.NewKeyFromSeed(b)
		return &PrivateKey{PublicKey: PublicKey{Type: protocol.SignatureTypeED25519, Key: key[32:]}, Key: key}, err

	case "FA": // Factom public
		b, err := parse1(s, sha256.Size, sha256.Size, 0x5f, 0xb1)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("invalid FA address: %w", err)
		}
		return &PublicKeyHash{Type: protocol.SignatureTypeRCD1, Hash: b}, nil

	case "Fs": // Factom private
		b, err := parse1(s, ed25519.SeedSize, ed25519.SeedSize, 0x64, 0x78)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("invalid Fs address: %w", err)
		}
		key := ed25519.NewKeyFromSeed(b)
		return &PrivateKey{PublicKey: PublicKey{Type: protocol.SignatureTypeRCD1, Key: key[32:]}, Key: key}, err

	case "BT": // Bitcoin public
		b, err := parse1(s[2:], 20, sha256.Size, 0x00)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("invalid BTC address: %w", err)
		}
		return &PublicKeyHash{Type: protocol.SignatureTypeBTC, Hash: b}, nil

	case "0x": // Ethereum public
		b, err := hex.DecodeString(s[2:])
		if err != nil {
			return nil, errors.BadRequest.WithFormat("invalid ETH address: %w", err)
		}
		if len(b) != 20 {
			return nil, errors.BadRequest.WithFormat("invalid ETH address: want 20 bytes, got %d", len(b))
		}
		return &PublicKeyHash{Type: protocol.SignatureTypeETH, Hash: b}, nil

	case "MH": // Unknown hash (as a multihash)
		return parseMH(s)
	}

	// Raw hex - could be a hash or a key
	b, err := hex.DecodeString(s)
	if err == nil {
		return &Unknown{Value: b, Encoding: multibase.Base16}, nil
	}

	// Raw base58 - could be Bitcoin or a hash or a key
	b, err = base58.Decode(s)
	if err == nil {
		return &Unknown{Value: b, Encoding: multibase.Base58BTC}, nil
	}

	// Unprefixed lite account
	b, err = hex.DecodeString(strings.SplitN(s, "/", 2)[0])
	if err == nil {
		return parseLite(s)
	}

	return nil, errors.BadRequest.With("unknown address format")
}

func parseLite(s string) (Address, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, errors.BadRequest.With("invalid lite address: %w", err)
	}
	b, err := protocol.ParseLiteAddress(u)
	if err != nil {
		return nil, errors.BadRequest.With("invalid lite address: %w", err)
	}
	if len(b) != 20 {
		return nil, errors.BadRequest.WithFormat("invalid lite address: want 20 bytes (excluding checksum), got %d", len(b))
	}
	return &Lite{Url: u, Bytes: b}, nil
}

func parseMH(s string) (Address, error) {
	// Check the prefix
	if !strings.HasPrefix(s, "MH") {
		return nil, errors.BadRequest.With("invalid MH address: bad prefix")
	}

	// Decode
	_, b, err := multibase.Decode(s[2:])
	if err != nil {
		return nil, errors.BadRequest.WithFormat("invalid MH address: %v", err)
	}

	// Verify the checksum
	c := make([]byte, len(b)-2)
	copy(c, "MH")
	copy(c[2:], b)
	checksum := sha256.Sum256(c)
	checksum = sha256.Sum256(checksum[:])
	if !bytes.Equal(b[len(b)-4:], checksum[:4]) {
		return nil, errors.BadRequest.With("invalid MH address: bad checksum")
	}

	mh, err := multihash.Decode(b[:len(b)-4])
	if err != nil {
		return nil, errors.BadRequest.WithFormat("invalid MH address: %w", err)
	}
	return (*UnknownMultihash)(mh), nil
}

func parse1(s string, min, max int, prefix ...byte) ([]byte, error) {
	// Decode
	b, err := base58.Decode(s)
	if err != nil {
		return nil, errors.BadRequest.Wrap(err)
	}

	// Check the prefix
	if !bytes.HasPrefix(b, prefix) {
		return nil, errors.BadRequest.With("bad prefix")
	}

	// Check the length
	switch {
	case min == max && len(b) != len(prefix)+min+4:
		return nil, errors.BadRequest.WithFormat("want %d bytes, got %d", len(prefix)+min+4, len(b))
	case len(b) < len(prefix)+min+4:
		return nil, errors.BadRequest.WithFormat("want at least %d bytes, got %d", len(prefix)+min+4, len(b))
	case len(b) > len(prefix)+max+4:
		return nil, errors.BadRequest.WithFormat("want at most %d bytes, got %d", len(prefix)+max+4, len(b))
	}

	// Verify the checksum
	checksum := sha256.Sum256(b[:len(prefix)+min])
	checksum = sha256.Sum256(checksum[:])
	if !bytes.Equal(b[len(prefix)+min:], checksum[:4]) {
		return nil, errors.BadRequest.With("bad checksum")
	}

	return b[len(prefix) : len(prefix)+min], nil
}

func parse2(s string, bytelen int, prefix string) ([]byte, error) {
	// Check the prefix
	if !strings.HasPrefix(s, prefix) {
		return nil, errors.BadRequest.With("bad prefix")
	}

	// Decode
	b, err := base58.Decode(s[len(prefix):])
	if err != nil {
		return nil, errors.BadRequest.Wrap(err)
	}

	// Check the length
	if len(b) != bytelen+4 {
		return nil, errors.BadRequest.WithFormat("want %d bytes, got %d", bytelen+4, len(b))
	}

	// Verify the checksum
	c := make([]byte, len(prefix)+bytelen)
	n := copy(c, []byte(prefix))
	copy(c[n:], b)
	checksum := sha256.Sum256(c)
	checksum = sha256.Sum256(checksum[:])
	if !bytes.Equal(b[bytelen:], checksum[:4]) {
		return nil, errors.BadRequest.With("bad checksum")
	}

	return b[:bytelen], nil
}

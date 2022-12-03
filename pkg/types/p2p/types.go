package p2p

import (
	"encoding/json"
	"io"

	"github.com/multiformats/go-multiaddr"
)

// Multiaddr is a wrapper for the canonical multiaddr type.
type Multiaddr = multiaddr.Multiaddr

// CopyMultiaddr copies a multiaddr.
func CopyMultiaddr(v Multiaddr) Multiaddr {
	return v // No need to copy (immutable)
}

// EqualMultiaddr checks if two multiaddrs are equal.
func EqualMultiaddr(a, b Multiaddr) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(b)
}

// UnmarshalMultiaddr reads a multiaddr from the given bytes.
func UnmarshalMultiaddr(b []byte) (Multiaddr, error) {
	return multiaddr.NewMultiaddrBytes(b)
}

// UnmarshalMultiaddrFrom reads a multiaddr from the given reader.
func UnmarshalMultiaddrFrom(r io.Reader) (Multiaddr, error) {
	b, err := io.ReadAll(io.LimitReader(r, 1024))
	if err != nil {
		return nil, err
	}
	return multiaddr.NewMultiaddrBytes(b)
}

// UnmarshalMultiaddrJSON reads a multiaddr as a string from the given JSON
// bytes.
func UnmarshalMultiaddrJSON(b []byte) (Multiaddr, error) {
	var v string
	err := json.Unmarshal(b, &v)
	if err != nil {
		return nil, err
	}
	return multiaddr.NewMultiaddr(v)
}
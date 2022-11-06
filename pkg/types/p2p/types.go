package p2p

import (
	"encoding/json"
	"io"

	"github.com/multiformats/go-multiaddr"
)

type Multiaddr = multiaddr.Multiaddr

func CopyMultiaddr(v Multiaddr) Multiaddr {
	return v // No need to copy (immutable)
}

func EqualMultiaddr(a, b Multiaddr) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(b)
}

func UnmarshalMultiaddr(b []byte) (Multiaddr, error) {
	return multiaddr.NewMultiaddrBytes(b)
}

func UnmarshalMultiaddrFrom(r io.Reader) (Multiaddr, error) {
	b, err := io.ReadAll(io.LimitReader(r, 1024))
	if err != nil {
		return nil, err
	}
	return multiaddr.NewMultiaddrBytes(b)
}

func UnmarshalMultiaddrJSON(b []byte) (Multiaddr, error) {
	var v string
	err := json.Unmarshal(b, &v)
	if err != nil {
		return nil, err
	}
	return multiaddr.NewMultiaddr(v)
}

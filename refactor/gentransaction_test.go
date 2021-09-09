package refactor

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"
)

var Seed = sha256.Sum256([]byte{1, 2, 3})

// GetKey
// Get a private key (where the first 32 bytes is the private key, the second is the public key)
func GetKey() []byte {
	Seed = sha256.Sum256(Seed[:])
	privateKey := ed25519.NewKeyFromSeed(Seed[:])
	return privateKey
}

func TestTokenTransaction(t *testing.T) {
	testurl := "acc://0x411abc253de31674f"
	trans := new(GenTransaction)
	u, err := url.Parse(testurl)
	if err != nil {
		t.Error(err)
	}
	host := strings.ToLower(u.Host)
	h := sha256.Sum256([]byte(host))
	trans.Routing = uint64(h[0])<<56 | uint64(h[1])<<48 | uint64(h[2])<<40 | uint64(h[3])<<32 |
		uint64(h[4])<<24 | uint64(h[5])<<16 | uint64(h[6])<<8 | uint64(h[7])
	h = sha256.Sum256([]byte(testurl))
	trans.ChainID = h[:]
	trans.Transaction = []byte("this is a message to who ever is about")
	th := sha256.Sum256(trans.MarshalBinary())
	s := Sign(GetKey(), th[:])
	trans.Signature = append(trans.Signature, s)

	data := trans.Marshal()
	to := new(GenTransaction)
	to.UnMarshal(data)

	if !to.ValidateSig() {
		t.Error("failed to validate signature")
	}
	to.Routing++
	if to.ValidateSig() {
		t.Error("failed to invalidate signature")
	}
}

func TestDistribution(t *testing.T) {
	var routes [256]uint64

	cnt := uint64(29)
	seed := sha256.Sum256([]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1})

	for i := 0; i < 100000; i++ {
		privatekey := ed25519.NewKeyFromSeed(seed[:])
		x := privatekey[32:]
		route := uint64(x[0])<<56 | uint64(x[1])<<48 | uint64(x[2])<<40 | uint64(x[3])<<32 |
			uint64(x[4])<<24 | uint64(x[5])<<16 | uint64(x[6])<<8 | uint64(x[7])
		routes[route%cnt]++
		seed = sha256.Sum256(seed[:])
	}

	sort.Slice(routes[:], func(i, j int) bool {
		return routes[i] > routes[j]
	})
	for i, v := range routes[:cnt] {
		fmt.Printf("%3d %8d %6.4f%%\n", i, v, (float64(routes[i])-float64(routes[cnt-1]))/float64(routes[cnt-1])*100)
	}
}

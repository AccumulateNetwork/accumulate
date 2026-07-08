// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types

import (
	"crypto/ed25519"
	"testing"
)

// TestHeaderTimestampRoundTrip verifies a signed header survives the
// header-in-certificate wire round trip after the Timestamp field was added
// to the header format (#4054).
func TestHeaderTimestampRoundTrip(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)

	h := NewHeader(pub, 3, 0, []PayloadEntry{{Digest: BatchDigest{1, 2, 3}, Worker: 7}}, []CertificateDigest{{9}})
	h.Timestamp = 1234567890123456789
	if err := h.Sign(priv); err != nil {
		t.Fatal(err)
	}
	if err := h.Verify(); err != nil {
		t.Fatalf("verify original: %v", err)
	}

	// Header-only round trip
	hd, err := h.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	h2, err := UnmarshalHeader(hd)
	if err != nil {
		t.Fatal(err)
	}
	if h2.Timestamp != h.Timestamp {
		t.Fatalf("timestamp lost: %d != %d", h2.Timestamp, h.Timestamp)
	}
	if err := h2.Verify(); err != nil {
		t.Fatalf("verify after header round trip: %v", err)
	}

	// Header-in-certificate round trip
	sig := ed25519.Sign(priv, func() []byte {
		d := h.Digest()
		v := NewVote(d, h.Round, h.Epoch, pub)
		_ = v
		return d[:]
	}())
	cert := NewCertificate(h, [][]byte{sig}, []uint16{0})
	cd, err := cert.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	cert2, err := UnmarshalCertificate(cd)
	if err != nil {
		t.Fatal(err)
	}
	if cert2.Header.Timestamp != h.Timestamp {
		t.Fatalf("timestamp lost in cert: %d != %d", cert2.Header.Timestamp, h.Timestamp)
	}
	if err := cert2.Header.Verify(); err != nil {
		t.Fatalf("verify header after cert round trip: %v", err)
	}
}

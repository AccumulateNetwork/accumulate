// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package peerdb

import (
	"encoding/json"
	"math"
	"strings"
	"time"
)

//go:generate go run gitlab.com/accumulatenetwork/accumulate/tools/cmd/gen-types --package peerdb types.yml

func (s *PeerStatus) Compare(q *PeerStatus) int {
	return strings.Compare(s.ID.String(), q.ID.String())
}

func (s *PeerAddressStatus) Compare(b *PeerAddressStatus) int {
	return strings.Compare(s.Address.String(), b.Address.String())
}

func (s *PeerNetworkStatus) Compare(b *PeerNetworkStatus) int {
	return strings.Compare(strings.ToLower(s.Name), strings.ToLower(b.Name))
}

func (s *PeerServiceStatus) Compare(q *PeerServiceStatus) int {
	return s.Address.Compare(q.Address)
}

func (s *PeerStatus) UnmarshalJSON(b []byte) error {
	type T PeerStatus
	err := json.Unmarshal(b, (*T)(s))
	if err != nil {
		return err
	}
	if s.Addresses == nil {
		s.Addresses = new(AtomicSlice[*PeerAddressStatus, PeerAddressStatus])
	}
	if s.Networks == nil {
		s.Networks = new(AtomicSlice[*PeerNetworkStatus, PeerNetworkStatus])
	}
	return nil
}

func (s *PeerNetworkStatus) UnmarshalJSON(b []byte) error {
	type T PeerNetworkStatus
	err := json.Unmarshal(b, (*T)(s))
	if err != nil {
		return err
	}
	if s.Services == nil {
		s.Services = new(AtomicSlice[*PeerServiceStatus, PeerServiceStatus])
	}
	return nil
}

func (s *LastStatus) UnmarshalJSON(b []byte) error {
	type T LastStatus
	err := json.Unmarshal(b, (*T)(s))
	if err != nil {
		return err
	}
	if s.Failed == nil {
		s.Failed = new(AtomicUint)
	}
	return nil
}

func (l *LastStatus) DidAttempt() {
	now := time.Now()
	l.Attempt = &now
}

func (l *LastStatus) DidSucceed() {
	now := time.Now()
	l.Success = &now
	l.Failed.Store(0)
}

func (l *LastStatus) SinceAttempt() time.Duration {
	if l.Attempt == nil {
		return math.MaxInt64
	}
	return time.Since(*l.Attempt)
}

func (l *LastStatus) SinceSuccess() time.Duration {
	if l.Success == nil {
		return math.MaxInt64
	}
	return time.Since(*l.Success)
}

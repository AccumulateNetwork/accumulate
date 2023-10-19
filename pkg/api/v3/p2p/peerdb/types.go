// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package peerdb

import (
	"encoding/json"
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

func (l *LastStatus) DidAttempt() {
	now := time.Now()
	l.Attempt = &now
}

func (l *LastStatus) DidSucceed() {
	now := time.Now()
	l.Success = &now
}

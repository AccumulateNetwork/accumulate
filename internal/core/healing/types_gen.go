// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package healing

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

//lint:file-ignore S1001,S1002,S1008,SA4013 generated code

import (
	"encoding/json"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type PeerInfo struct {
	ID        p2p.PeerID
	Operator  *url.URL             `json:"operator,omitempty" form:"operator" query:"operator" validate:"required"`
	Key       [32]byte             `json:"key,omitempty" form:"key" query:"key" validate:"required"`
	Status    *api.ConsensusStatus `json:"status,omitempty" form:"status" query:"status" validate:"required"`
	Addresses []p2p.Multiaddr      `json:"addresses,omitempty" form:"addresses" query:"addresses" validate:"required"`
}

func (v *PeerInfo) Copy() *PeerInfo {
	u := new(PeerInfo)

	if v.Operator != nil {
		u.Operator = v.Operator
	}
	u.Key = v.Key
	if v.Status != nil {
		u.Status = (v.Status).Copy()
	}
	u.Addresses = make([]p2p.Multiaddr, len(v.Addresses))
	for i, v := range v.Addresses {
		v := v
		if v != nil {
			u.Addresses[i] = p2p.CopyMultiaddr(v)
		}
	}

	return u
}

func (v *PeerInfo) CopyAsInterface() interface{} { return v.Copy() }

func (v *PeerInfo) Equal(u *PeerInfo) bool {
	switch {
	case v.Operator == u.Operator:
		// equal
	case v.Operator == nil || u.Operator == nil:
		return false
	case !((v.Operator).Equal(u.Operator)):
		return false
	}
	if !(v.Key == u.Key) {
		return false
	}
	switch {
	case v.Status == u.Status:
		// equal
	case v.Status == nil || u.Status == nil:
		return false
	case !((v.Status).Equal(u.Status)):
		return false
	}
	if len(v.Addresses) != len(u.Addresses) {
		return false
	}
	for i := range v.Addresses {
		if !(p2p.EqualMultiaddr(v.Addresses[i], u.Addresses[i])) {
			return false
		}
	}

	return true
}

func (v *PeerInfo) MarshalJSON() ([]byte, error) {
	u := struct {
		Operator  *url.URL                                       `json:"operator,omitempty"`
		Key       *string                                        `json:"key,omitempty"`
		Status    *api.ConsensusStatus                           `json:"status,omitempty"`
		Addresses *encoding.JsonUnmarshalListWith[p2p.Multiaddr] `json:"addresses,omitempty"`
	}{}
	if !(v.Operator == nil) {
		u.Operator = v.Operator
	}
	if !(v.Key == ([32]byte{})) {
		u.Key = encoding.ChainToJSON(&v.Key)
	}
	if !(v.Status == nil) {
		u.Status = v.Status
	}
	if !(len(v.Addresses) == 0) {
		u.Addresses = &encoding.JsonUnmarshalListWith[p2p.Multiaddr]{Value: v.Addresses, Func: p2p.UnmarshalMultiaddrJSON}
	}
	return json.Marshal(&u)
}

func (v *PeerInfo) UnmarshalJSON(data []byte) error {
	u := struct {
		Operator  *url.URL                                       `json:"operator,omitempty"`
		Key       *string                                        `json:"key,omitempty"`
		Status    *api.ConsensusStatus                           `json:"status,omitempty"`
		Addresses *encoding.JsonUnmarshalListWith[p2p.Multiaddr] `json:"addresses,omitempty"`
	}{}
	u.Operator = v.Operator
	u.Key = encoding.ChainToJSON(&v.Key)
	u.Status = v.Status
	u.Addresses = &encoding.JsonUnmarshalListWith[p2p.Multiaddr]{Value: v.Addresses, Func: p2p.UnmarshalMultiaddrJSON}
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Operator = u.Operator
	if x, err := encoding.ChainFromJSON(u.Key); err != nil {
		return fmt.Errorf("error decoding Key: %w", err)
	} else {
		v.Key = *x
	}
	v.Status = u.Status
	if u.Addresses != nil {
		v.Addresses = make([]p2p.Multiaddr, len(u.Addresses.Value))
		for i, x := range u.Addresses.Value {
			v.Addresses[i] = x
		}
	}
	return nil
}

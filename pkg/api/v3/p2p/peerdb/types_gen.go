// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package peerdb

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

//lint:file-ignore S1001,S1002,S1008,SA4013 generated code

import (
	"encoding/json"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/p2p"
)

type LastStatus struct {
	Success *time.Time `json:"success,omitempty" form:"success" query:"success" validate:"required"`
	Attempt *time.Time `json:"attempt,omitempty" form:"attempt" query:"attempt" validate:"required"`
}

type PeerAddressStatus struct {
	Address p2p.Multiaddr `json:"address,omitempty" form:"address" query:"address" validate:"required"`
	Last    LastStatus    `json:"last,omitempty" form:"last" query:"last" validate:"required"`
}

type PeerNetworkStatus struct {
	Name     string                                              `json:"name,omitempty" form:"name" query:"name" validate:"required"`
	Services *AtomicSlice[*PeerServiceStatus, PeerServiceStatus] `json:"services,omitempty" form:"services" query:"services" validate:"required"`
}

type PeerServiceStatus struct {
	Address *api.ServiceAddress `json:"address,omitempty" form:"address" query:"address" validate:"required"`
	Last    LastStatus          `json:"last,omitempty" form:"last" query:"last" validate:"required"`
}

type PeerStatus struct {
	ID        p2p.PeerID
	Addresses *AtomicSlice[*PeerAddressStatus, PeerAddressStatus] `json:"addresses,omitempty" form:"addresses" query:"addresses" validate:"required"`
	Networks  *AtomicSlice[*PeerNetworkStatus, PeerNetworkStatus] `json:"networks,omitempty" form:"networks" query:"networks" validate:"required"`
}

func (v *LastStatus) Copy() *LastStatus {
	u := new(LastStatus)

	if v.Success != nil {
		u.Success = new(time.Time)
		*u.Success = *v.Success
	}
	if v.Attempt != nil {
		u.Attempt = new(time.Time)
		*u.Attempt = *v.Attempt
	}

	return u
}

func (v *LastStatus) CopyAsInterface() interface{} { return v.Copy() }

func (v *PeerAddressStatus) Copy() *PeerAddressStatus {
	u := new(PeerAddressStatus)

	if v.Address != nil {
		u.Address = p2p.CopyMultiaddr(v.Address)
	}
	u.Last = *(&v.Last).Copy()

	return u
}

func (v *PeerAddressStatus) CopyAsInterface() interface{} { return v.Copy() }

func (v *PeerNetworkStatus) Copy() *PeerNetworkStatus {
	u := new(PeerNetworkStatus)

	u.Name = v.Name
	if v.Services != nil {
		u.Services = (v.Services).Copy()
	}

	return u
}

func (v *PeerNetworkStatus) CopyAsInterface() interface{} { return v.Copy() }

func (v *PeerServiceStatus) Copy() *PeerServiceStatus {
	u := new(PeerServiceStatus)

	if v.Address != nil {
		u.Address = (v.Address).Copy()
	}
	u.Last = *(&v.Last).Copy()

	return u
}

func (v *PeerServiceStatus) CopyAsInterface() interface{} { return v.Copy() }

func (v *PeerStatus) Copy() *PeerStatus {
	u := new(PeerStatus)

	if v.Addresses != nil {
		u.Addresses = (v.Addresses).Copy()
	}
	if v.Networks != nil {
		u.Networks = (v.Networks).Copy()
	}

	return u
}

func (v *PeerStatus) CopyAsInterface() interface{} { return v.Copy() }

func (v *LastStatus) Equal(u *LastStatus) bool {
	switch {
	case v.Success == u.Success:
		// equal
	case v.Success == nil || u.Success == nil:
		return false
	case !((*v.Success).Equal(*u.Success)):
		return false
	}
	switch {
	case v.Attempt == u.Attempt:
		// equal
	case v.Attempt == nil || u.Attempt == nil:
		return false
	case !((*v.Attempt).Equal(*u.Attempt)):
		return false
	}

	return true
}

func (v *PeerAddressStatus) Equal(u *PeerAddressStatus) bool {
	if !(p2p.EqualMultiaddr(v.Address, u.Address)) {
		return false
	}
	if !((&v.Last).Equal(&u.Last)) {
		return false
	}

	return true
}

func (v *PeerNetworkStatus) Equal(u *PeerNetworkStatus) bool {
	if !(v.Name == u.Name) {
		return false
	}
	switch {
	case v.Services == u.Services:
		// equal
	case v.Services == nil || u.Services == nil:
		return false
	case !((v.Services).Equal(u.Services)):
		return false
	}

	return true
}

func (v *PeerServiceStatus) Equal(u *PeerServiceStatus) bool {
	switch {
	case v.Address == u.Address:
		// equal
	case v.Address == nil || u.Address == nil:
		return false
	case !((v.Address).Equal(u.Address)):
		return false
	}
	if !((&v.Last).Equal(&u.Last)) {
		return false
	}

	return true
}

func (v *PeerStatus) Equal(u *PeerStatus) bool {
	switch {
	case v.Addresses == u.Addresses:
		// equal
	case v.Addresses == nil || u.Addresses == nil:
		return false
	case !((v.Addresses).Equal(u.Addresses)):
		return false
	}
	switch {
	case v.Networks == u.Networks:
		// equal
	case v.Networks == nil || u.Networks == nil:
		return false
	case !((v.Networks).Equal(u.Networks)):
		return false
	}

	return true
}

func (v *PeerAddressStatus) MarshalJSON() ([]byte, error) {
	u := struct {
		Address *encoding.JsonUnmarshalWith[p2p.Multiaddr] `json:"address,omitempty"`
		Last    LastStatus                                 `json:"last,omitempty"`
	}{}
	if !(p2p.EqualMultiaddr(v.Address, nil)) {
		u.Address = &encoding.JsonUnmarshalWith[p2p.Multiaddr]{Value: v.Address, Func: p2p.UnmarshalMultiaddrJSON}
	}
	if !((v.Last).Equal(new(LastStatus))) {
		u.Last = v.Last
	}
	return json.Marshal(&u)
}

func (v *PeerAddressStatus) UnmarshalJSON(data []byte) error {
	u := struct {
		Address *encoding.JsonUnmarshalWith[p2p.Multiaddr] `json:"address,omitempty"`
		Last    LastStatus                                 `json:"last,omitempty"`
	}{}
	u.Address = &encoding.JsonUnmarshalWith[p2p.Multiaddr]{Value: v.Address, Func: p2p.UnmarshalMultiaddrJSON}
	u.Last = v.Last
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	if u.Address != nil {
		v.Address = u.Address.Value
	}

	v.Last = u.Last
	return nil
}

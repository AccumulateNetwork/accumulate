// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

//lint:file-ignore S1001,S1002,S1008,SA4013 generated code

import (
	"encoding/json"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

type BvnInit struct {
	Id    string      `json:"id,omitempty" form:"id" query:"id" validate:"required"`
	Nodes []*NodeInit `json:"nodes,omitempty" form:"nodes" query:"nodes" validate:"required"`
}

type NetworkInit struct {
	Id   string     `json:"id,omitempty" form:"id" query:"id" validate:"required"`
	Bvns []*BvnInit `json:"bvns,omitempty" form:"bvns" query:"bvns" validate:"required"`
}

type NodeInit struct {
	DnnType          config.NodeType `json:"dnnType,omitempty" form:"dnnType" query:"dnnType" validate:"required"`
	BvnnType         config.NodeType `json:"bvnnType,omitempty" form:"bvnnType" query:"bvnnType" validate:"required"`
	BasePort         uint64          `json:"basePort,omitempty" form:"basePort" query:"basePort" validate:"required"`
	AdvertizeAddress string          `json:"advertizeAddress,omitempty" form:"advertizeAddress" query:"advertizeAddress" validate:"required"`
	ListenAddress    string          `json:"listenAddress,omitempty" form:"listenAddress" query:"listenAddress" validate:"required"`
	PeerAddress      string          `json:"peerAddress,omitempty" form:"peerAddress" query:"peerAddress" validate:"required"`
	PrivValKey       []byte          `json:"privValKey,omitempty" form:"privValKey" query:"privValKey" validate:"required"`
	DnNodeKey        []byte          `json:"dnNodeKey,omitempty" form:"dnNodeKey" query:"dnNodeKey" validate:"required"`
	BvnNodeKey       []byte          `json:"bvnNodeKey,omitempty" form:"bvnNodeKey" query:"bvnNodeKey" validate:"required"`
}

func (v *BvnInit) MarshalJSON() ([]byte, error) {
	u := struct {
		Id    string                       `json:"id,omitempty"`
		Nodes encoding.JsonList[*NodeInit] `json:"nodes,omitempty"`
	}{}
	if !(len(v.Id) == 0) {
		u.Id = v.Id
	}
	if !(len(v.Nodes) == 0) {
		u.Nodes = v.Nodes
	}
	return json.Marshal(&u)
}

func (v *NetworkInit) MarshalJSON() ([]byte, error) {
	u := struct {
		Id   string                      `json:"id,omitempty"`
		Bvns encoding.JsonList[*BvnInit] `json:"bvns,omitempty"`
	}{}
	if !(len(v.Id) == 0) {
		u.Id = v.Id
	}
	if !(len(v.Bvns) == 0) {
		u.Bvns = v.Bvns
	}
	return json.Marshal(&u)
}

func (v *NodeInit) MarshalJSON() ([]byte, error) {
	u := struct {
		DnnType          config.NodeType `json:"dnnType,omitempty"`
		BvnnType         config.NodeType `json:"bvnnType,omitempty"`
		BasePort         uint64          `json:"basePort,omitempty"`
		AdvertizeAddress string          `json:"advertizeAddress,omitempty"`
		HostName         string          `json:"hostName,omitempty"`
		ListenAddress    string          `json:"listenAddress,omitempty"`
		ListenIP         string          `json:"listenIP,omitempty"`
		PeerAddress      string          `json:"peerAddress,omitempty"`
		PrivValKey       *string         `json:"privValKey,omitempty"`
		DnNodeKey        *string         `json:"dnNodeKey,omitempty"`
		BvnNodeKey       *string         `json:"bvnNodeKey,omitempty"`
	}{}
	if !(v.DnnType == 0) {
		u.DnnType = v.DnnType
	}
	if !(v.BvnnType == 0) {
		u.BvnnType = v.BvnnType
	}
	if !(v.BasePort == 0) {
		u.BasePort = v.BasePort
	}
	if !(len(v.AdvertizeAddress) == 0) {
		u.AdvertizeAddress = v.AdvertizeAddress
		u.HostName = v.AdvertizeAddress
	}
	if !(len(v.ListenAddress) == 0) {
		u.ListenAddress = v.ListenAddress
		u.ListenIP = v.ListenAddress
	}
	if !(len(v.PeerAddress) == 0) {
		u.PeerAddress = v.PeerAddress
	}
	if !(len(v.PrivValKey) == 0) {
		u.PrivValKey = encoding.BytesToJSON(v.PrivValKey)
	}
	if !(len(v.DnNodeKey) == 0) {
		u.DnNodeKey = encoding.BytesToJSON(v.DnNodeKey)
	}
	if !(len(v.BvnNodeKey) == 0) {
		u.BvnNodeKey = encoding.BytesToJSON(v.BvnNodeKey)
	}
	return json.Marshal(&u)
}

func (v *BvnInit) UnmarshalJSON(data []byte) error {
	u := struct {
		Id    string                       `json:"id,omitempty"`
		Nodes encoding.JsonList[*NodeInit] `json:"nodes,omitempty"`
	}{}
	u.Id = v.Id
	u.Nodes = v.Nodes
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Id = u.Id
	v.Nodes = u.Nodes
	return nil
}

func (v *NetworkInit) UnmarshalJSON(data []byte) error {
	u := struct {
		Id   string                      `json:"id,omitempty"`
		Bvns encoding.JsonList[*BvnInit] `json:"bvns,omitempty"`
	}{}
	u.Id = v.Id
	u.Bvns = v.Bvns
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Id = u.Id
	v.Bvns = u.Bvns
	return nil
}

func (v *NodeInit) UnmarshalJSON(data []byte) error {
	u := struct {
		DnnType          config.NodeType `json:"dnnType,omitempty"`
		BvnnType         config.NodeType `json:"bvnnType,omitempty"`
		BasePort         uint64          `json:"basePort,omitempty"`
		AdvertizeAddress string          `json:"advertizeAddress,omitempty"`
		HostName         string          `json:"hostName,omitempty"`
		ListenAddress    string          `json:"listenAddress,omitempty"`
		ListenIP         string          `json:"listenIP,omitempty"`
		PeerAddress      string          `json:"peerAddress,omitempty"`
		PrivValKey       *string         `json:"privValKey,omitempty"`
		DnNodeKey        *string         `json:"dnNodeKey,omitempty"`
		BvnNodeKey       *string         `json:"bvnNodeKey,omitempty"`
	}{}
	u.DnnType = v.DnnType
	u.BvnnType = v.BvnnType
	u.BasePort = v.BasePort
	u.AdvertizeAddress = v.AdvertizeAddress
	u.HostName = v.AdvertizeAddress
	u.ListenAddress = v.ListenAddress
	u.ListenIP = v.ListenAddress
	u.PeerAddress = v.PeerAddress
	u.PrivValKey = encoding.BytesToJSON(v.PrivValKey)
	u.DnNodeKey = encoding.BytesToJSON(v.DnNodeKey)
	u.BvnNodeKey = encoding.BytesToJSON(v.BvnNodeKey)
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.DnnType = u.DnnType
	v.BvnnType = u.BvnnType
	v.BasePort = u.BasePort
	if !(u.AdvertizeAddress == "") {
		v.AdvertizeAddress = u.AdvertizeAddress
	} else {
		v.AdvertizeAddress = u.HostName
	}
	if !(u.ListenAddress == "") {
		v.ListenAddress = u.ListenAddress
	} else {
		v.ListenAddress = u.ListenIP
	}
	v.PeerAddress = u.PeerAddress
	if x, err := encoding.BytesFromJSON(u.PrivValKey); err != nil {
		return fmt.Errorf("error decoding PrivValKey: %w", err)
	} else {
		v.PrivValKey = x
	}
	if x, err := encoding.BytesFromJSON(u.DnNodeKey); err != nil {
		return fmt.Errorf("error decoding DnNodeKey: %w", err)
	} else {
		v.DnNodeKey = x
	}
	if x, err := encoding.BytesFromJSON(u.BvnNodeKey); err != nil {
		return fmt.Errorf("error decoding BvnNodeKey: %w", err)
	} else {
		v.BvnNodeKey = x
	}
	return nil
}

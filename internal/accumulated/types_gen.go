package accumulated

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

import (
	"encoding/json"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
)

type BvnInit struct {
	Id        string      `json:"id,omitempty" form:"id" query:"id" validate:"required"`
	Nodes     []*NodeInit `json:"nodes,omitempty" form:"nodes" query:"nodes" validate:"required"`
	extraData []byte
}

type NetworkInit struct {
	Id        string     `json:"id,omitempty" form:"id" query:"id" validate:"required"`
	Bvns      []*BvnInit `json:"bvns,omitempty" form:"bvns" query:"bvns" validate:"required"`
	extraData []byte
}

type NodeInit struct {
	DnnType    config.NodeType `json:"dnnType,omitempty" form:"dnnType" query:"dnnType" validate:"required"`
	BvnnType   config.NodeType `json:"bvnnType,omitempty" form:"bvnnType" query:"bvnnType" validate:"required"`
	BasePort   uint64          `json:"basePort,omitempty" form:"basePort" query:"basePort" validate:"required"`
	HostName   string          `json:"hostName,omitempty" form:"hostName" query:"hostName" validate:"required"`
	ListenIP   string          `json:"listenIP,omitempty" form:"listenIP" query:"listenIP" validate:"required"`
	PrivValKey []byte          `json:"privValKey,omitempty" form:"privValKey" query:"privValKey" validate:"required"`
	NodeKey    []byte          `json:"nodeKey,omitempty" form:"nodeKey" query:"nodeKey" validate:"required"`
	extraData  []byte
}

func (v *BvnInit) MarshalJSON() ([]byte, error) {
	u := struct {
		Id    string                       `json:"id,omitempty"`
		Nodes encoding.JsonList[*NodeInit] `json:"nodes,omitempty"`
	}{}
	u.Id = v.Id
	u.Nodes = v.Nodes
	return json.Marshal(&u)
}

func (v *NetworkInit) MarshalJSON() ([]byte, error) {
	u := struct {
		Id   string                      `json:"id,omitempty"`
		Bvns encoding.JsonList[*BvnInit] `json:"bvns,omitempty"`
	}{}
	u.Id = v.Id
	u.Bvns = v.Bvns
	return json.Marshal(&u)
}

func (v *NodeInit) MarshalJSON() ([]byte, error) {
	u := struct {
		DnnType    config.NodeType `json:"dnnType,omitempty"`
		BvnnType   config.NodeType `json:"bvnnType,omitempty"`
		BasePort   uint64          `json:"basePort,omitempty"`
		HostName   string          `json:"hostName,omitempty"`
		ListenIP   string          `json:"listenIP,omitempty"`
		PrivValKey *string         `json:"privValKey,omitempty"`
		NodeKey    *string         `json:"nodeKey,omitempty"`
	}{}
	u.DnnType = v.DnnType
	u.BvnnType = v.BvnnType
	u.BasePort = v.BasePort
	u.HostName = v.HostName
	u.ListenIP = v.ListenIP
	u.PrivValKey = encoding.BytesToJSON(v.PrivValKey)
	u.NodeKey = encoding.BytesToJSON(v.NodeKey)
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
		DnnType    config.NodeType `json:"dnnType,omitempty"`
		BvnnType   config.NodeType `json:"bvnnType,omitempty"`
		BasePort   uint64          `json:"basePort,omitempty"`
		HostName   string          `json:"hostName,omitempty"`
		ListenIP   string          `json:"listenIP,omitempty"`
		PrivValKey *string         `json:"privValKey,omitempty"`
		NodeKey    *string         `json:"nodeKey,omitempty"`
	}{}
	u.DnnType = v.DnnType
	u.BvnnType = v.BvnnType
	u.BasePort = v.BasePort
	u.HostName = v.HostName
	u.ListenIP = v.ListenIP
	u.PrivValKey = encoding.BytesToJSON(v.PrivValKey)
	u.NodeKey = encoding.BytesToJSON(v.NodeKey)
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.DnnType = u.DnnType
	v.BvnnType = u.BvnnType
	v.BasePort = u.BasePort
	v.HostName = u.HostName
	v.ListenIP = u.ListenIP
	if x, err := encoding.BytesFromJSON(u.PrivValKey); err != nil {
		return fmt.Errorf("error decoding PrivValKey: %w", err)
	} else {
		v.PrivValKey = x
	}
	if x, err := encoding.BytesFromJSON(u.NodeKey); err != nil {
		return fmt.Errorf("error decoding NodeKey: %w", err)
	} else {
		v.NodeKey = x
	}
	return nil
}

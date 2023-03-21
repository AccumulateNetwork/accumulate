// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package config

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

//lint:file-ignore S1001,S1002,S1008,SA4013 generated code

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/p2p"
)

type Describe struct {
	fieldsSet    []bool
	NetworkType  NetworkType `json:"networkType,omitempty" form:"networkType" query:"networkType" validate:"required" toml:"type" mapstructure:"type"`
	PartitionId  string      `json:"partitionId,omitempty" form:"partitionId" query:"partitionId" validate:"required" toml:"partition-id" mapstructure:"partition-id"`
	LocalAddress string      `json:"localAddress,omitempty" form:"localAddress" query:"localAddress" validate:"required" toml:"local-address" mapstructure:"local-address"`
	Network      Network     `json:"network,omitempty" form:"network" query:"network" validate:"required" toml:"network" mapstructure:"network"`
	extraData    []byte
}

type Network struct {
	fieldsSet  []bool
	Id         string      `json:"id,omitempty" form:"id" query:"id" validate:"required" toml:"id" mapstructure:"id"`
	Partitions []Partition `json:"partitions,omitempty" form:"partitions" query:"partitions" validate:"required" toml:"partitions" mapstructure:"partitions"`
	extraData  []byte
}

type Node struct {
	fieldsSet []bool
	Address   string   `json:"address,omitempty" form:"address" query:"address" validate:"required" toml:"address" mapstructure:"address"`
	Type      NodeType `json:"type,omitempty" form:"type" query:"type" validate:"required" toml:"type" mapstructure:"type"`
	extraData []byte
}

type P2P struct {
	fieldsSet      []bool
	Listen         []p2p.Multiaddr `json:"listen,omitempty" form:"listen" query:"listen" validate:"required" toml:"listen" mapstructure:"listen"`
	BootstrapPeers []p2p.Multiaddr `json:"bootstrapPeers,omitempty" form:"bootstrapPeers" query:"bootstrapPeers" validate:"required" toml:"bootstrap-peers" mapstructure:"bootstrap-peers"`
	extraData      []byte
}

type Partition struct {
	fieldsSet []bool
	Id        string      `json:"id,omitempty" form:"id" query:"id" validate:"required" toml:"id" mapstructure:"id"`
	Type      NetworkType `json:"type,omitempty" form:"type" query:"type" validate:"required" toml:"type" mapstructure:"type"`
	BasePort  int64       `json:"basePort,omitempty" form:"basePort" query:"basePort" validate:"required" toml:"port" mapstructure:"port"`
	Nodes     []Node      `json:"nodes,omitempty" form:"nodes" query:"nodes" validate:"required" toml:"nodes" mapstructure:"nodes"`
	extraData []byte
}

func (v *Describe) Copy() *Describe {
	u := new(Describe)

	u.NetworkType = v.NetworkType
	u.PartitionId = v.PartitionId
	u.LocalAddress = v.LocalAddress
	u.Network = *(&v.Network).Copy()

	return u
}

func (v *Describe) CopyAsInterface() interface{} { return v.Copy() }

func (v *Network) Copy() *Network {
	u := new(Network)

	u.Id = v.Id
	u.Partitions = make([]Partition, len(v.Partitions))
	for i, v := range v.Partitions {
		u.Partitions[i] = *(&v).Copy()
	}

	return u
}

func (v *Network) CopyAsInterface() interface{} { return v.Copy() }

func (v *Node) Copy() *Node {
	u := new(Node)

	u.Address = v.Address
	u.Type = v.Type

	return u
}

func (v *Node) CopyAsInterface() interface{} { return v.Copy() }

func (v *P2P) Copy() *P2P {
	u := new(P2P)

	u.Listen = make([]p2p.Multiaddr, len(v.Listen))
	for i, v := range v.Listen {
		if v != nil {
			u.Listen[i] = p2p.CopyMultiaddr(v)
		}
	}
	u.BootstrapPeers = make([]p2p.Multiaddr, len(v.BootstrapPeers))
	for i, v := range v.BootstrapPeers {
		if v != nil {
			u.BootstrapPeers[i] = p2p.CopyMultiaddr(v)
		}
	}

	return u
}

func (v *P2P) CopyAsInterface() interface{} { return v.Copy() }

func (v *Partition) Copy() *Partition {
	u := new(Partition)

	u.Id = v.Id
	u.Type = v.Type
	u.BasePort = v.BasePort
	u.Nodes = make([]Node, len(v.Nodes))
	for i, v := range v.Nodes {
		u.Nodes[i] = *(&v).Copy()
	}

	return u
}

func (v *Partition) CopyAsInterface() interface{} { return v.Copy() }

func (v *Describe) Equal(u *Describe) bool {
	if !(v.NetworkType == u.NetworkType) {
		return false
	}
	if !(v.PartitionId == u.PartitionId) {
		return false
	}
	if !(v.LocalAddress == u.LocalAddress) {
		return false
	}
	if !((&v.Network).Equal(&u.Network)) {
		return false
	}

	return true
}

func (v *Network) Equal(u *Network) bool {
	if !(v.Id == u.Id) {
		return false
	}
	if len(v.Partitions) != len(u.Partitions) {
		return false
	}
	for i := range v.Partitions {
		if !((&v.Partitions[i]).Equal(&u.Partitions[i])) {
			return false
		}
	}

	return true
}

func (v *Node) Equal(u *Node) bool {
	if !(v.Address == u.Address) {
		return false
	}
	if !(v.Type == u.Type) {
		return false
	}

	return true
}

func (v *P2P) Equal(u *P2P) bool {
	if len(v.Listen) != len(u.Listen) {
		return false
	}
	for i := range v.Listen {
		if !(p2p.EqualMultiaddr(v.Listen[i], u.Listen[i])) {
			return false
		}
	}
	if len(v.BootstrapPeers) != len(u.BootstrapPeers) {
		return false
	}
	for i := range v.BootstrapPeers {
		if !(p2p.EqualMultiaddr(v.BootstrapPeers[i], u.BootstrapPeers[i])) {
			return false
		}
	}

	return true
}

func (v *Partition) Equal(u *Partition) bool {
	if !(v.Id == u.Id) {
		return false
	}
	if !(v.Type == u.Type) {
		return false
	}
	if !(v.BasePort == u.BasePort) {
		return false
	}
	if len(v.Nodes) != len(u.Nodes) {
		return false
	}
	for i := range v.Nodes {
		if !((&v.Nodes[i]).Equal(&u.Nodes[i])) {
			return false
		}
	}

	return true
}

var fieldNames_Describe = []string{
	1: "NetworkType",
	2: "PartitionId",
	3: "LocalAddress",
	4: "Network",
}

func (v *Describe) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.NetworkType == 0) {
		writer.WriteEnum(1, v.NetworkType)
	}
	if !(len(v.PartitionId) == 0) {
		writer.WriteString(2, v.PartitionId)
	}
	if !(len(v.LocalAddress) == 0) {
		writer.WriteString(3, v.LocalAddress)
	}
	if !((v.Network).Equal(new(Network))) {
		writer.WriteValue(4, v.Network.MarshalBinary)
	}

	_, _, err := writer.Reset(fieldNames_Describe)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *Describe) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field NetworkType is missing")
	} else if v.NetworkType == 0 {
		errs = append(errs, "field NetworkType is not set")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field PartitionId is missing")
	} else if len(v.PartitionId) == 0 {
		errs = append(errs, "field PartitionId is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field LocalAddress is missing")
	} else if len(v.LocalAddress) == 0 {
		errs = append(errs, "field LocalAddress is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field Network is missing")
	} else if (v.Network).Equal(new(Network)) {
		errs = append(errs, "field Network is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

var fieldNames_Network = []string{
	1: "Id",
	2: "Partitions",
}

func (v *Network) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Id) == 0) {
		writer.WriteString(1, v.Id)
	}
	if !(len(v.Partitions) == 0) {
		for _, v := range v.Partitions {
			writer.WriteValue(2, v.MarshalBinary)
		}
	}

	_, _, err := writer.Reset(fieldNames_Network)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *Network) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Id is missing")
	} else if len(v.Id) == 0 {
		errs = append(errs, "field Id is not set")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Partitions is missing")
	} else if len(v.Partitions) == 0 {
		errs = append(errs, "field Partitions is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

var fieldNames_Node = []string{
	1: "Address",
	2: "Type",
}

func (v *Node) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Address) == 0) {
		writer.WriteString(1, v.Address)
	}
	if !(v.Type == 0) {
		writer.WriteEnum(2, v.Type)
	}

	_, _, err := writer.Reset(fieldNames_Node)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *Node) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Address is missing")
	} else if len(v.Address) == 0 {
		errs = append(errs, "field Address is not set")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Type is missing")
	} else if v.Type == 0 {
		errs = append(errs, "field Type is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

var fieldNames_P2P = []string{
	1: "Listen",
	2: "BootstrapPeers",
}

func (v *P2P) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Listen) == 0) {
		for _, v := range v.Listen {
			writer.WriteValue(1, v.MarshalBinary)
		}
	}
	if !(len(v.BootstrapPeers) == 0) {
		for _, v := range v.BootstrapPeers {
			writer.WriteValue(2, v.MarshalBinary)
		}
	}

	_, _, err := writer.Reset(fieldNames_P2P)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *P2P) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Listen is missing")
	} else if len(v.Listen) == 0 {
		errs = append(errs, "field Listen is not set")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field BootstrapPeers is missing")
	} else if len(v.BootstrapPeers) == 0 {
		errs = append(errs, "field BootstrapPeers is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

var fieldNames_Partition = []string{
	1: "Id",
	2: "Type",
	3: "BasePort",
	4: "Nodes",
}

func (v *Partition) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Id) == 0) {
		writer.WriteString(1, v.Id)
	}
	if !(v.Type == 0) {
		writer.WriteEnum(2, v.Type)
	}
	if !(v.BasePort == 0) {
		writer.WriteInt(3, v.BasePort)
	}
	if !(len(v.Nodes) == 0) {
		for _, v := range v.Nodes {
			writer.WriteValue(4, v.MarshalBinary)
		}
	}

	_, _, err := writer.Reset(fieldNames_Partition)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *Partition) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Id is missing")
	} else if len(v.Id) == 0 {
		errs = append(errs, "field Id is not set")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Type is missing")
	} else if v.Type == 0 {
		errs = append(errs, "field Type is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field BasePort is missing")
	} else if v.BasePort == 0 {
		errs = append(errs, "field BasePort is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field Nodes is missing")
	} else if len(v.Nodes) == 0 {
		errs = append(errs, "field Nodes is not set")
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

func (v *Describe) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *Describe) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x := new(NetworkType); reader.ReadEnum(1, x) {
		v.NetworkType = *x
	}
	if x, ok := reader.ReadString(2); ok {
		v.PartitionId = x
	}
	if x, ok := reader.ReadString(3); ok {
		v.LocalAddress = x
	}
	if x := new(Network); reader.ReadValue(4, x.UnmarshalBinaryFrom) {
		v.Network = *x
	}

	seen, err := reader.Reset(fieldNames_Describe)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func (v *Network) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *Network) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadString(1); ok {
		v.Id = x
	}
	for {
		if x := new(Partition); reader.ReadValue(2, x.UnmarshalBinaryFrom) {
			v.Partitions = append(v.Partitions, *x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_Network)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func (v *Node) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *Node) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadString(1); ok {
		v.Address = x
	}
	if x := new(NodeType); reader.ReadEnum(2, x) {
		v.Type = *x
	}

	seen, err := reader.Reset(fieldNames_Node)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func (v *P2P) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *P2P) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	for {
		ok := reader.ReadValue(1, func(r io.Reader) error {
			x, err := p2p.UnmarshalMultiaddrFrom(r)
			if err == nil {
				v.Listen = append(v.Listen, x)
			}
			return err
		})
		if !ok {
			break
		}
	}
	for {
		ok := reader.ReadValue(2, func(r io.Reader) error {
			x, err := p2p.UnmarshalMultiaddrFrom(r)
			if err == nil {
				v.BootstrapPeers = append(v.BootstrapPeers, x)
			}
			return err
		})
		if !ok {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_P2P)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func (v *Partition) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *Partition) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadString(1); ok {
		v.Id = x
	}
	if x := new(NetworkType); reader.ReadEnum(2, x) {
		v.Type = *x
	}
	if x, ok := reader.ReadInt(3); ok {
		v.BasePort = x
	}
	for {
		if x := new(Node); reader.ReadValue(4, x.UnmarshalBinaryFrom) {
			v.Nodes = append(v.Nodes, *x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_Partition)
	if err != nil {
		return encoding.Error{E: err}
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	if err != nil {
		return encoding.Error{E: err}
	}
	return nil
}

func (v *Describe) MarshalJSON() ([]byte, error) {
	u := struct {
		NetworkType  NetworkType `json:"networkType,omitempty"`
		PartitionId  string      `json:"partitionId,omitempty"`
		SubnetId     string      `json:"subnetId,omitempty"`
		LocalAddress string      `json:"localAddress,omitempty"`
		Network      Network     `json:"network,omitempty"`
	}{}
	if !(v.NetworkType == 0) {
		u.NetworkType = v.NetworkType
	}
	if !(len(v.PartitionId) == 0) {
		u.PartitionId = v.PartitionId
		u.SubnetId = v.PartitionId
	}
	if !(len(v.LocalAddress) == 0) {
		u.LocalAddress = v.LocalAddress
	}
	if !((v.Network).Equal(new(Network))) {
		u.Network = v.Network
	}
	return json.Marshal(&u)
}

func (v *Network) MarshalJSON() ([]byte, error) {
	u := struct {
		Id         string                       `json:"id,omitempty"`
		Partitions encoding.JsonList[Partition] `json:"partitions,omitempty"`
		Subnets    encoding.JsonList[Partition] `json:"subnets,omitempty"`
	}{}
	if !(len(v.Id) == 0) {
		u.Id = v.Id
	}
	if !(len(v.Partitions) == 0) {
		u.Partitions = v.Partitions
		u.Subnets = v.Partitions
	}
	return json.Marshal(&u)
}

func (v *P2P) MarshalJSON() ([]byte, error) {
	u := struct {
		Listen         *encoding.JsonUnmarshalListWith[p2p.Multiaddr] `json:"listen,omitempty"`
		BootstrapPeers *encoding.JsonUnmarshalListWith[p2p.Multiaddr] `json:"bootstrapPeers,omitempty"`
	}{}
	if !(len(v.Listen) == 0) {
		u.Listen = &encoding.JsonUnmarshalListWith[p2p.Multiaddr]{Value: v.Listen, Func: p2p.UnmarshalMultiaddrJSON}
	}
	if !(len(v.BootstrapPeers) == 0) {
		u.BootstrapPeers = &encoding.JsonUnmarshalListWith[p2p.Multiaddr]{Value: v.BootstrapPeers, Func: p2p.UnmarshalMultiaddrJSON}
	}
	return json.Marshal(&u)
}

func (v *Partition) MarshalJSON() ([]byte, error) {
	u := struct {
		Id       string                  `json:"id,omitempty"`
		Type     NetworkType             `json:"type,omitempty"`
		BasePort int64                   `json:"basePort,omitempty"`
		Nodes    encoding.JsonList[Node] `json:"nodes,omitempty"`
	}{}
	if !(len(v.Id) == 0) {
		u.Id = v.Id
	}
	if !(v.Type == 0) {
		u.Type = v.Type
	}
	if !(v.BasePort == 0) {
		u.BasePort = v.BasePort
	}
	if !(len(v.Nodes) == 0) {
		u.Nodes = v.Nodes
	}
	return json.Marshal(&u)
}

func (v *Describe) UnmarshalJSON(data []byte) error {
	u := struct {
		NetworkType  NetworkType `json:"networkType,omitempty"`
		PartitionId  string      `json:"partitionId,omitempty"`
		SubnetId     string      `json:"subnetId,omitempty"`
		LocalAddress string      `json:"localAddress,omitempty"`
		Network      Network     `json:"network,omitempty"`
	}{}
	u.NetworkType = v.NetworkType
	u.PartitionId = v.PartitionId
	u.SubnetId = v.PartitionId
	u.LocalAddress = v.LocalAddress
	u.Network = v.Network
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.NetworkType = u.NetworkType
	if !(u.PartitionId == "") {
		v.PartitionId = u.PartitionId
	} else {
		v.PartitionId = u.SubnetId
	}
	v.LocalAddress = u.LocalAddress
	v.Network = u.Network
	return nil
}

func (v *Network) UnmarshalJSON(data []byte) error {
	u := struct {
		Id         string                       `json:"id,omitempty"`
		Partitions encoding.JsonList[Partition] `json:"partitions,omitempty"`
		Subnets    encoding.JsonList[Partition] `json:"subnets,omitempty"`
	}{}
	u.Id = v.Id
	u.Partitions = v.Partitions
	u.Subnets = v.Partitions
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Id = u.Id
	if !(len(u.Partitions) == 0) {
		v.Partitions = u.Partitions
	} else {
		v.Partitions = u.Subnets
	}
	return nil
}

func (v *P2P) UnmarshalJSON(data []byte) error {
	u := struct {
		Listen         *encoding.JsonUnmarshalListWith[p2p.Multiaddr] `json:"listen,omitempty"`
		BootstrapPeers *encoding.JsonUnmarshalListWith[p2p.Multiaddr] `json:"bootstrapPeers,omitempty"`
	}{}
	u.Listen = &encoding.JsonUnmarshalListWith[p2p.Multiaddr]{Value: v.Listen, Func: p2p.UnmarshalMultiaddrJSON}
	u.BootstrapPeers = &encoding.JsonUnmarshalListWith[p2p.Multiaddr]{Value: v.BootstrapPeers, Func: p2p.UnmarshalMultiaddrJSON}
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	if u.Listen != nil {
		v.Listen = make([]p2p.Multiaddr, len(u.Listen.Value))
		for i, x := range u.Listen.Value {
			v.Listen[i] = x
		}
	}
	if u.BootstrapPeers != nil {
		v.BootstrapPeers = make([]p2p.Multiaddr, len(u.BootstrapPeers.Value))
		for i, x := range u.BootstrapPeers.Value {
			v.BootstrapPeers[i] = x
		}
	}
	return nil
}

func (v *Partition) UnmarshalJSON(data []byte) error {
	u := struct {
		Id       string                  `json:"id,omitempty"`
		Type     NetworkType             `json:"type,omitempty"`
		BasePort int64                   `json:"basePort,omitempty"`
		Nodes    encoding.JsonList[Node] `json:"nodes,omitempty"`
	}{}
	u.Id = v.Id
	u.Type = v.Type
	u.BasePort = v.BasePort
	u.Nodes = v.Nodes
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Id = u.Id
	v.Type = u.Type
	v.BasePort = u.BasePort
	v.Nodes = u.Nodes
	return nil
}

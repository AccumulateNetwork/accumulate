// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package proxy

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

//lint:file-ignore S1001,S1002,S1008,SA4013 generated code

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type NetworkConfigRequest struct {
	Network string `json:"network,omitempty" form:"network" query:"network" validate:"required"`
	Sign    bool   `json:"sign,omitempty" form:"sign" query:"sign"`
}

type NetworkConfigResponse struct {
	NetworkState NetworkState          `json:"networkState,omitempty" form:"networkState" query:"networkState" validate:"required"`
	Signature    protocol.KeySignature `json:"signature,omitempty" form:"signature" query:"signature"`
}

type NetworkState struct {
	fieldsSet      []bool
	Network        config.Network `json:"network,omitempty" form:"network" query:"network" validate:"required"`
	Version        string         `json:"version,omitempty" form:"version" query:"version" validate:"required"`
	Commit         string         `json:"commit,omitempty" form:"commit" query:"commit" validate:"required"`
	VersionIsKnown bool           `json:"versionIsKnown,omitempty" form:"versionIsKnown" query:"versionIsKnown" validate:"required"`
	IsTestNet      bool           `json:"isTestNet,omitempty" form:"isTestNet" query:"isTestNet" validate:"required"`
	extraData      []byte
}

type PartitionList struct {
	fieldsSet  []bool
	Partitions []string `json:"partitions,omitempty" form:"partitions" query:"partitions" validate:"required"`
	extraData  []byte
}

type PartitionListRequest struct {
	Network string `json:"network,omitempty" form:"network" query:"network" validate:"required"`
	Sign    bool   `json:"sign,omitempty" form:"sign" query:"sign"`
}

type PartitionListResponse struct {
	PartitionList
	Signature protocol.KeySignature `json:"signature,omitempty" form:"signature" query:"signature"`
}

type SeedCount struct {
	fieldsSet []bool
	Count     int64 `json:"count,omitempty" form:"count" query:"count" validate:"required"`
	extraData []byte
}

type SeedCountRequest struct {
	Network   string `json:"network,omitempty" form:"network" query:"network" validate:"required"`
	Partition string `json:"partition,omitempty" form:"partition" query:"partition" validate:"required"`
	Sign      bool   `json:"sign,omitempty" form:"sign" query:"sign"`
}

type SeedCountResponse struct {
	SeedCount
	Signature protocol.KeySignature `json:"signature,omitempty" form:"signature" query:"signature"`
}

type SeedList struct {
	fieldsSet []bool
	BasePort  uint64             `json:"basePort,omitempty" form:"basePort" query:"basePort" validate:"required"`
	Type      config.NetworkType `json:"type,omitempty" form:"type" query:"type" validate:"required"`
	Addresses []string           `json:"addresses,omitempty" form:"addresses" query:"addresses" validate:"required"`
	extraData []byte
}

type SeedListRequest struct {
	Network   string `json:"network,omitempty" form:"network" query:"network" validate:"required"`
	Partition string `json:"partition,omitempty" form:"partition" query:"partition" validate:"required"`
	Count     int64  `json:"count,omitempty" form:"count" query:"count" validate:"required"`
	Sign      bool   `json:"sign,omitempty" form:"sign" query:"sign"`
}

type SeedListResponse struct {
	SeedList
	Signature protocol.KeySignature `json:"signature,omitempty" form:"signature" query:"signature"`
}

func (v *NetworkConfigRequest) Copy() *NetworkConfigRequest {
	u := new(NetworkConfigRequest)

	u.Network = v.Network
	u.Sign = v.Sign

	return u
}

func (v *NetworkConfigRequest) CopyAsInterface() interface{} { return v.Copy() }

func (v *NetworkConfigResponse) Copy() *NetworkConfigResponse {
	u := new(NetworkConfigResponse)

	u.NetworkState = *(&v.NetworkState).Copy()
	if v.Signature != nil {
		u.Signature = protocol.CopyKeySignature(v.Signature)
	}

	return u
}

func (v *NetworkConfigResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *NetworkState) Copy() *NetworkState {
	u := new(NetworkState)

	u.Network = *(&v.Network).Copy()
	u.Version = v.Version
	u.Commit = v.Commit
	u.VersionIsKnown = v.VersionIsKnown
	u.IsTestNet = v.IsTestNet

	return u
}

func (v *NetworkState) CopyAsInterface() interface{} { return v.Copy() }

func (v *PartitionList) Copy() *PartitionList {
	u := new(PartitionList)

	u.Partitions = make([]string, len(v.Partitions))
	for i, v := range v.Partitions {
		u.Partitions[i] = v
	}

	return u
}

func (v *PartitionList) CopyAsInterface() interface{} { return v.Copy() }

func (v *PartitionListRequest) Copy() *PartitionListRequest {
	u := new(PartitionListRequest)

	u.Network = v.Network
	u.Sign = v.Sign

	return u
}

func (v *PartitionListRequest) CopyAsInterface() interface{} { return v.Copy() }

func (v *PartitionListResponse) Copy() *PartitionListResponse {
	u := new(PartitionListResponse)

	u.PartitionList = *v.PartitionList.Copy()
	if v.Signature != nil {
		u.Signature = protocol.CopyKeySignature(v.Signature)
	}

	return u
}

func (v *PartitionListResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *SeedCount) Copy() *SeedCount {
	u := new(SeedCount)

	u.Count = v.Count

	return u
}

func (v *SeedCount) CopyAsInterface() interface{} { return v.Copy() }

func (v *SeedCountRequest) Copy() *SeedCountRequest {
	u := new(SeedCountRequest)

	u.Network = v.Network
	u.Partition = v.Partition
	u.Sign = v.Sign

	return u
}

func (v *SeedCountRequest) CopyAsInterface() interface{} { return v.Copy() }

func (v *SeedCountResponse) Copy() *SeedCountResponse {
	u := new(SeedCountResponse)

	u.SeedCount = *v.SeedCount.Copy()
	if v.Signature != nil {
		u.Signature = protocol.CopyKeySignature(v.Signature)
	}

	return u
}

func (v *SeedCountResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *SeedList) Copy() *SeedList {
	u := new(SeedList)

	u.BasePort = v.BasePort
	u.Type = v.Type
	u.Addresses = make([]string, len(v.Addresses))
	for i, v := range v.Addresses {
		u.Addresses[i] = v
	}

	return u
}

func (v *SeedList) CopyAsInterface() interface{} { return v.Copy() }

func (v *SeedListRequest) Copy() *SeedListRequest {
	u := new(SeedListRequest)

	u.Network = v.Network
	u.Partition = v.Partition
	u.Count = v.Count
	u.Sign = v.Sign

	return u
}

func (v *SeedListRequest) CopyAsInterface() interface{} { return v.Copy() }

func (v *SeedListResponse) Copy() *SeedListResponse {
	u := new(SeedListResponse)

	u.SeedList = *v.SeedList.Copy()
	if v.Signature != nil {
		u.Signature = protocol.CopyKeySignature(v.Signature)
	}

	return u
}

func (v *SeedListResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *NetworkConfigRequest) Equal(u *NetworkConfigRequest) bool {
	if !(v.Network == u.Network) {
		return false
	}
	if !(v.Sign == u.Sign) {
		return false
	}

	return true
}

func (v *NetworkConfigResponse) Equal(u *NetworkConfigResponse) bool {
	if !((&v.NetworkState).Equal(&u.NetworkState)) {
		return false
	}
	if !(protocol.EqualKeySignature(v.Signature, u.Signature)) {
		return false
	}

	return true
}

func (v *NetworkState) Equal(u *NetworkState) bool {
	if !((&v.Network).Equal(&u.Network)) {
		return false
	}
	if !(v.Version == u.Version) {
		return false
	}
	if !(v.Commit == u.Commit) {
		return false
	}
	if !(v.VersionIsKnown == u.VersionIsKnown) {
		return false
	}
	if !(v.IsTestNet == u.IsTestNet) {
		return false
	}

	return true
}

func (v *PartitionList) Equal(u *PartitionList) bool {
	if len(v.Partitions) != len(u.Partitions) {
		return false
	}
	for i := range v.Partitions {
		if !(v.Partitions[i] == u.Partitions[i]) {
			return false
		}
	}

	return true
}

func (v *PartitionListRequest) Equal(u *PartitionListRequest) bool {
	if !(v.Network == u.Network) {
		return false
	}
	if !(v.Sign == u.Sign) {
		return false
	}

	return true
}

func (v *PartitionListResponse) Equal(u *PartitionListResponse) bool {
	if !v.PartitionList.Equal(&u.PartitionList) {
		return false
	}
	if !(protocol.EqualKeySignature(v.Signature, u.Signature)) {
		return false
	}

	return true
}

func (v *SeedCount) Equal(u *SeedCount) bool {
	if !(v.Count == u.Count) {
		return false
	}

	return true
}

func (v *SeedCountRequest) Equal(u *SeedCountRequest) bool {
	if !(v.Network == u.Network) {
		return false
	}
	if !(v.Partition == u.Partition) {
		return false
	}
	if !(v.Sign == u.Sign) {
		return false
	}

	return true
}

func (v *SeedCountResponse) Equal(u *SeedCountResponse) bool {
	if !v.SeedCount.Equal(&u.SeedCount) {
		return false
	}
	if !(protocol.EqualKeySignature(v.Signature, u.Signature)) {
		return false
	}

	return true
}

func (v *SeedList) Equal(u *SeedList) bool {
	if !(v.BasePort == u.BasePort) {
		return false
	}
	if !(v.Type == u.Type) {
		return false
	}
	if len(v.Addresses) != len(u.Addresses) {
		return false
	}
	for i := range v.Addresses {
		if !(v.Addresses[i] == u.Addresses[i]) {
			return false
		}
	}

	return true
}

func (v *SeedListRequest) Equal(u *SeedListRequest) bool {
	if !(v.Network == u.Network) {
		return false
	}
	if !(v.Partition == u.Partition) {
		return false
	}
	if !(v.Count == u.Count) {
		return false
	}
	if !(v.Sign == u.Sign) {
		return false
	}

	return true
}

func (v *SeedListResponse) Equal(u *SeedListResponse) bool {
	if !v.SeedList.Equal(&u.SeedList) {
		return false
	}
	if !(protocol.EqualKeySignature(v.Signature, u.Signature)) {
		return false
	}

	return true
}

var fieldNames_NetworkState = []string{
	1: "Network",
	2: "Version",
	3: "Commit",
	4: "VersionIsKnown",
	5: "IsTestNet",
}

func (v *NetworkState) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !((v.Network).Equal(new(config.Network))) {
		writer.WriteValue(1, v.Network.MarshalBinary)
	}
	if !(len(v.Version) == 0) {
		writer.WriteString(2, v.Version)
	}
	if !(len(v.Commit) == 0) {
		writer.WriteString(3, v.Commit)
	}
	if !(!v.VersionIsKnown) {
		writer.WriteBool(4, v.VersionIsKnown)
	}
	if !(!v.IsTestNet) {
		writer.WriteBool(5, v.IsTestNet)
	}

	_, _, err := writer.Reset(fieldNames_NetworkState)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *NetworkState) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Network is missing")
	} else if (v.Network).Equal(new(config.Network)) {
		errs = append(errs, "field Network is not set")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Version is missing")
	} else if len(v.Version) == 0 {
		errs = append(errs, "field Version is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Commit is missing")
	} else if len(v.Commit) == 0 {
		errs = append(errs, "field Commit is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field VersionIsKnown is missing")
	} else if !v.VersionIsKnown {
		errs = append(errs, "field VersionIsKnown is not set")
	}
	if len(v.fieldsSet) > 4 && !v.fieldsSet[4] {
		errs = append(errs, "field IsTestNet is missing")
	} else if !v.IsTestNet {
		errs = append(errs, "field IsTestNet is not set")
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

var fieldNames_PartitionList = []string{
	1: "Partitions",
}

func (v *PartitionList) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Partitions) == 0) {
		for _, v := range v.Partitions {
			writer.WriteString(1, v)
		}
	}

	_, _, err := writer.Reset(fieldNames_PartitionList)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *PartitionList) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
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

var fieldNames_SeedCount = []string{
	1: "Count",
}

func (v *SeedCount) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Count == 0) {
		writer.WriteInt(1, v.Count)
	}

	_, _, err := writer.Reset(fieldNames_SeedCount)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *SeedCount) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Count is missing")
	} else if v.Count == 0 {
		errs = append(errs, "field Count is not set")
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

var fieldNames_SeedList = []string{
	1: "BasePort",
	2: "Type",
	3: "Addresses",
}

func (v *SeedList) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.BasePort == 0) {
		writer.WriteUint(1, v.BasePort)
	}
	if !(v.Type == 0) {
		writer.WriteEnum(2, v.Type)
	}
	if !(len(v.Addresses) == 0) {
		for _, v := range v.Addresses {
			writer.WriteString(3, v)
		}
	}

	_, _, err := writer.Reset(fieldNames_SeedList)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *SeedList) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field BasePort is missing")
	} else if v.BasePort == 0 {
		errs = append(errs, "field BasePort is not set")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Type is missing")
	} else if v.Type == 0 {
		errs = append(errs, "field Type is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Addresses is missing")
	} else if len(v.Addresses) == 0 {
		errs = append(errs, "field Addresses is not set")
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

func (v *NetworkState) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *NetworkState) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x := new(config.Network); reader.ReadValue(1, x.UnmarshalBinaryFrom) {
		v.Network = *x
	}
	if x, ok := reader.ReadString(2); ok {
		v.Version = x
	}
	if x, ok := reader.ReadString(3); ok {
		v.Commit = x
	}
	if x, ok := reader.ReadBool(4); ok {
		v.VersionIsKnown = x
	}
	if x, ok := reader.ReadBool(5); ok {
		v.IsTestNet = x
	}

	seen, err := reader.Reset(fieldNames_NetworkState)
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

func (v *PartitionList) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *PartitionList) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	for {
		if x, ok := reader.ReadString(1); ok {
			v.Partitions = append(v.Partitions, x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_PartitionList)
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

func (v *SeedCount) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *SeedCount) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadInt(1); ok {
		v.Count = x
	}

	seen, err := reader.Reset(fieldNames_SeedCount)
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

func (v *SeedList) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *SeedList) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadUint(1); ok {
		v.BasePort = x
	}
	if x := new(config.NetworkType); reader.ReadEnum(2, x) {
		v.Type = *x
	}
	for {
		if x, ok := reader.ReadString(3); ok {
			v.Addresses = append(v.Addresses, x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_SeedList)
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

func (v *NetworkConfigResponse) MarshalJSON() ([]byte, error) {
	u := struct {
		NetworkState NetworkState                                       `json:"networkState,omitempty"`
		Signature    *encoding.JsonUnmarshalWith[protocol.KeySignature] `json:"signature,omitempty"`
	}{}
	if !((v.NetworkState).Equal(new(NetworkState))) {
		u.NetworkState = v.NetworkState
	}
	if !(protocol.EqualKeySignature(v.Signature, nil)) {
		u.Signature = &encoding.JsonUnmarshalWith[protocol.KeySignature]{Value: v.Signature, Func: protocol.UnmarshalKeySignatureJSON}
	}
	return json.Marshal(&u)
}

func (v *PartitionList) MarshalJSON() ([]byte, error) {
	u := struct {
		Partitions encoding.JsonList[string] `json:"partitions,omitempty"`
	}{}
	if !(len(v.Partitions) == 0) {
		u.Partitions = v.Partitions
	}
	return json.Marshal(&u)
}

func (v *PartitionListResponse) MarshalJSON() ([]byte, error) {
	u := struct {
		Partitions encoding.JsonList[string]                          `json:"partitions,omitempty"`
		Signature  *encoding.JsonUnmarshalWith[protocol.KeySignature] `json:"signature,omitempty"`
	}{}
	if !(len(v.PartitionList.Partitions) == 0) {

		u.Partitions = v.PartitionList.Partitions
	}
	if !(protocol.EqualKeySignature(v.Signature, nil)) {
		u.Signature = &encoding.JsonUnmarshalWith[protocol.KeySignature]{Value: v.Signature, Func: protocol.UnmarshalKeySignatureJSON}
	}
	return json.Marshal(&u)
}

func (v *SeedCountResponse) MarshalJSON() ([]byte, error) {
	u := struct {
		Count     int64                                              `json:"count,omitempty"`
		Signature *encoding.JsonUnmarshalWith[protocol.KeySignature] `json:"signature,omitempty"`
	}{}
	if !(v.SeedCount.Count == 0) {

		u.Count = v.SeedCount.Count
	}
	if !(protocol.EqualKeySignature(v.Signature, nil)) {
		u.Signature = &encoding.JsonUnmarshalWith[protocol.KeySignature]{Value: v.Signature, Func: protocol.UnmarshalKeySignatureJSON}
	}
	return json.Marshal(&u)
}

func (v *SeedList) MarshalJSON() ([]byte, error) {
	u := struct {
		BasePort  uint64                    `json:"basePort,omitempty"`
		Type      config.NetworkType        `json:"type,omitempty"`
		Addresses encoding.JsonList[string] `json:"addresses,omitempty"`
	}{}
	if !(v.BasePort == 0) {
		u.BasePort = v.BasePort
	}
	if !(v.Type == 0) {
		u.Type = v.Type
	}
	if !(len(v.Addresses) == 0) {
		u.Addresses = v.Addresses
	}
	return json.Marshal(&u)
}

func (v *SeedListResponse) MarshalJSON() ([]byte, error) {
	u := struct {
		BasePort  uint64                                             `json:"basePort,omitempty"`
		Type      config.NetworkType                                 `json:"type,omitempty"`
		Addresses encoding.JsonList[string]                          `json:"addresses,omitempty"`
		Signature *encoding.JsonUnmarshalWith[protocol.KeySignature] `json:"signature,omitempty"`
	}{}
	if !(v.SeedList.BasePort == 0) {

		u.BasePort = v.SeedList.BasePort
	}
	if !(v.SeedList.Type == 0) {

		u.Type = v.SeedList.Type
	}
	if !(len(v.SeedList.Addresses) == 0) {

		u.Addresses = v.SeedList.Addresses
	}
	if !(protocol.EqualKeySignature(v.Signature, nil)) {
		u.Signature = &encoding.JsonUnmarshalWith[protocol.KeySignature]{Value: v.Signature, Func: protocol.UnmarshalKeySignatureJSON}
	}
	return json.Marshal(&u)
}

func (v *NetworkConfigResponse) UnmarshalJSON(data []byte) error {
	u := struct {
		NetworkState NetworkState                                       `json:"networkState,omitempty"`
		Signature    *encoding.JsonUnmarshalWith[protocol.KeySignature] `json:"signature,omitempty"`
	}{}
	u.NetworkState = v.NetworkState
	u.Signature = &encoding.JsonUnmarshalWith[protocol.KeySignature]{Value: v.Signature, Func: protocol.UnmarshalKeySignatureJSON}
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.NetworkState = u.NetworkState
	if u.Signature != nil {
		v.Signature = u.Signature.Value
	}

	return nil
}

func (v *PartitionList) UnmarshalJSON(data []byte) error {
	u := struct {
		Partitions encoding.JsonList[string] `json:"partitions,omitempty"`
	}{}
	u.Partitions = v.Partitions
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Partitions = u.Partitions
	return nil
}

func (v *PartitionListResponse) UnmarshalJSON(data []byte) error {
	u := struct {
		Partitions encoding.JsonList[string]                          `json:"partitions,omitempty"`
		Signature  *encoding.JsonUnmarshalWith[protocol.KeySignature] `json:"signature,omitempty"`
	}{}
	u.Partitions = v.PartitionList.Partitions
	u.Signature = &encoding.JsonUnmarshalWith[protocol.KeySignature]{Value: v.Signature, Func: protocol.UnmarshalKeySignatureJSON}
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.PartitionList.Partitions = u.Partitions
	if u.Signature != nil {
		v.Signature = u.Signature.Value
	}

	return nil
}

func (v *SeedCountResponse) UnmarshalJSON(data []byte) error {
	u := struct {
		Count     int64                                              `json:"count,omitempty"`
		Signature *encoding.JsonUnmarshalWith[protocol.KeySignature] `json:"signature,omitempty"`
	}{}
	u.Count = v.SeedCount.Count
	u.Signature = &encoding.JsonUnmarshalWith[protocol.KeySignature]{Value: v.Signature, Func: protocol.UnmarshalKeySignatureJSON}
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.SeedCount.Count = u.Count
	if u.Signature != nil {
		v.Signature = u.Signature.Value
	}

	return nil
}

func (v *SeedList) UnmarshalJSON(data []byte) error {
	u := struct {
		BasePort  uint64                    `json:"basePort,omitempty"`
		Type      config.NetworkType        `json:"type,omitempty"`
		Addresses encoding.JsonList[string] `json:"addresses,omitempty"`
	}{}
	u.BasePort = v.BasePort
	u.Type = v.Type
	u.Addresses = v.Addresses
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.BasePort = u.BasePort
	v.Type = u.Type
	v.Addresses = u.Addresses
	return nil
}

func (v *SeedListResponse) UnmarshalJSON(data []byte) error {
	u := struct {
		BasePort  uint64                                             `json:"basePort,omitempty"`
		Type      config.NetworkType                                 `json:"type,omitempty"`
		Addresses encoding.JsonList[string]                          `json:"addresses,omitempty"`
		Signature *encoding.JsonUnmarshalWith[protocol.KeySignature] `json:"signature,omitempty"`
	}{}
	u.BasePort = v.SeedList.BasePort
	u.Type = v.SeedList.Type
	u.Addresses = v.SeedList.Addresses
	u.Signature = &encoding.JsonUnmarshalWith[protocol.KeySignature]{Value: v.Signature, Func: protocol.UnmarshalKeySignatureJSON}
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.SeedList.BasePort = u.BasePort
	v.SeedList.Type = u.Type
	v.SeedList.Addresses = u.Addresses
	if u.Signature != nil {
		v.Signature = u.Signature.Value
	}

	return nil
}

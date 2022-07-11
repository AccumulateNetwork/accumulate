package proxy

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type NetworkConfig struct {
	fieldsSet []bool
	Network   config.Network `json:"network,omitempty" form:"network" query:"network" validate:"required"`
	extraData []byte
}

type NetworkConfigRequest struct {
	Network   string `json:"network,omitempty" form:"network" query:"network" validate:"required"`
	Sign      bool   `json:"sign,omitempty" form:"sign" query:"sign"`
	extraData []byte
}

type NetworkConfigResponse struct {
	Network   config.Network        `json:"network,omitempty" form:"network" query:"network" validate:"required"`
	Signature protocol.KeySignature `json:"signature,omitempty" form:"signature" query:"signature"`
	extraData []byte
}

type PartitionList struct {
	fieldsSet  []bool
	Partitions []string `json:"partitions,omitempty" form:"partitions" query:"partitions" validate:"required"`
	extraData  []byte
}

type PartitionListRequest struct {
	Network   string `json:"network,omitempty" form:"network" query:"network" validate:"required"`
	Sign      bool   `json:"sign,omitempty" form:"sign" query:"sign"`
	extraData []byte
}

type PartitionListResponse struct {
	PartitionList
	Signature protocol.KeySignature `json:"signature,omitempty" form:"signature" query:"signature"`
	extraData []byte
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
	extraData []byte
}

type SeedCountResponse struct {
	SeedCount
	Signature protocol.KeySignature `json:"signature,omitempty" form:"signature" query:"signature"`
	extraData []byte
}

type SeedList struct {
	fieldsSet []bool
	Addresses []string `json:"addresses,omitempty" form:"addresses" query:"addresses" validate:"required"`
	extraData []byte
}

type SeedListRequest struct {
	Network   string `json:"network,omitempty" form:"network" query:"network" validate:"required"`
	Partition string `json:"partition,omitempty" form:"partition" query:"partition" validate:"required"`
	Count     int64  `json:"count,omitempty" form:"count" query:"count" validate:"required"`
	Sign      bool   `json:"sign,omitempty" form:"sign" query:"sign"`
	extraData []byte
}

type SeedListResponse struct {
	SeedList
	Signature protocol.KeySignature `json:"signature,omitempty" form:"signature" query:"signature"`
	extraData []byte
}

func (v *NetworkConfig) Copy() *NetworkConfig {
	u := new(NetworkConfig)

	u.Network = *(&v.Network).Copy()

	return u
}

func (v *NetworkConfig) CopyAsInterface() interface{} { return v.Copy() }

func (v *NetworkConfigRequest) Copy() *NetworkConfigRequest {
	u := new(NetworkConfigRequest)

	u.Network = v.Network
	u.Sign = v.Sign

	return u
}

func (v *NetworkConfigRequest) CopyAsInterface() interface{} { return v.Copy() }

func (v *NetworkConfigResponse) Copy() *NetworkConfigResponse {
	u := new(NetworkConfigResponse)

	u.Network = *(&v.Network).Copy()
	if v.Signature != nil {
		u.Signature = (v.Signature).CopyAsInterface().(protocol.KeySignature)
	}

	return u
}

func (v *NetworkConfigResponse) CopyAsInterface() interface{} { return v.Copy() }

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
		u.Signature = (v.Signature).CopyAsInterface().(protocol.KeySignature)
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
		u.Signature = (v.Signature).CopyAsInterface().(protocol.KeySignature)
	}

	return u
}

func (v *SeedCountResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *SeedList) Copy() *SeedList {
	u := new(SeedList)

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
		u.Signature = (v.Signature).CopyAsInterface().(protocol.KeySignature)
	}

	return u
}

func (v *SeedListResponse) CopyAsInterface() interface{} { return v.Copy() }

func (v *NetworkConfig) Equal(u *NetworkConfig) bool {
	if !((&v.Network).Equal(&u.Network)) {
		return false
	}

	return true
}

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
	if !((&v.Network).Equal(&u.Network)) {
		return false
	}
	if !(protocol.EqualKeySignature(v.Signature, u.Signature)) {
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

var fieldNames_NetworkConfig = []string{
	1: "Network",
}

func (v *NetworkConfig) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !((v.Network).Equal(new(config.Network))) {
		writer.WriteValue(1, v.Network.MarshalBinary)
	}

	_, _, err := writer.Reset(fieldNames_NetworkConfig)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *NetworkConfig) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Network is missing")
	} else if (v.Network).Equal(new(config.Network)) {
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

var fieldNames_PartitionList = []string{
	1: "Partitions",
}

func (v *PartitionList) MarshalBinary() ([]byte, error) {
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

var fieldNames_SeedCount = []string{
	1: "Count",
}

func (v *SeedCount) MarshalBinary() ([]byte, error) {
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

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
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
	1: "Addresses",
}

func (v *SeedList) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Addresses) == 0) {
		for _, v := range v.Addresses {
			writer.WriteString(1, v)
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

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
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

func (v *NetworkConfig) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *NetworkConfig) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x := new(config.Network); reader.ReadValue(1, x.UnmarshalBinary) {
		v.Network = *x
	}

	seen, err := reader.Reset(fieldNames_NetworkConfig)
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

	for {
		if x, ok := reader.ReadString(1); ok {
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
		Network   config.Network                                    `json:"network,omitempty"`
		Signature encoding.JsonUnmarshalWith[protocol.KeySignature] `json:"signature,omitempty"`
	}{}
	u.Network = v.Network
	u.Signature = encoding.JsonUnmarshalWith[protocol.KeySignature]{Value: v.Signature, Func: protocol.UnmarshalKeySignatureJSON}
	return json.Marshal(&u)
}

func (v *PartitionList) MarshalJSON() ([]byte, error) {
	u := struct {
		Partitions encoding.JsonList[string] `json:"partitions,omitempty"`
	}{}
	u.Partitions = v.Partitions
	return json.Marshal(&u)
}

func (v *PartitionListResponse) MarshalJSON() ([]byte, error) {
	u := struct {
		Partitions encoding.JsonList[string]                         `json:"partitions,omitempty"`
		Signature  encoding.JsonUnmarshalWith[protocol.KeySignature] `json:"signature,omitempty"`
	}{}
	u.Partitions = v.PartitionList.Partitions
	u.Signature = encoding.JsonUnmarshalWith[protocol.KeySignature]{Value: v.Signature, Func: protocol.UnmarshalKeySignatureJSON}
	return json.Marshal(&u)
}

func (v *SeedCountResponse) MarshalJSON() ([]byte, error) {
	u := struct {
		Count     int64                                             `json:"count,omitempty"`
		Signature encoding.JsonUnmarshalWith[protocol.KeySignature] `json:"signature,omitempty"`
	}{}
	u.Count = v.SeedCount.Count
	u.Signature = encoding.JsonUnmarshalWith[protocol.KeySignature]{Value: v.Signature, Func: protocol.UnmarshalKeySignatureJSON}
	return json.Marshal(&u)
}

func (v *SeedList) MarshalJSON() ([]byte, error) {
	u := struct {
		Addresses encoding.JsonList[string] `json:"addresses,omitempty"`
	}{}
	u.Addresses = v.Addresses
	return json.Marshal(&u)
}

func (v *SeedListResponse) MarshalJSON() ([]byte, error) {
	u := struct {
		Addresses encoding.JsonList[string]                         `json:"addresses,omitempty"`
		Signature encoding.JsonUnmarshalWith[protocol.KeySignature] `json:"signature,omitempty"`
	}{}
	u.Addresses = v.SeedList.Addresses
	u.Signature = encoding.JsonUnmarshalWith[protocol.KeySignature]{Value: v.Signature, Func: protocol.UnmarshalKeySignatureJSON}
	return json.Marshal(&u)
}

func (v *NetworkConfigResponse) UnmarshalJSON(data []byte) error {
	u := struct {
		Network   config.Network                                    `json:"network,omitempty"`
		Signature encoding.JsonUnmarshalWith[protocol.KeySignature] `json:"signature,omitempty"`
	}{}
	u.Network = v.Network
	u.Signature = encoding.JsonUnmarshalWith[protocol.KeySignature]{Value: v.Signature, Func: protocol.UnmarshalKeySignatureJSON}
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Network = u.Network
	v.Signature = u.Signature.Value

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
		Partitions encoding.JsonList[string]                         `json:"partitions,omitempty"`
		Signature  encoding.JsonUnmarshalWith[protocol.KeySignature] `json:"signature,omitempty"`
	}{}
	u.Partitions = v.PartitionList.Partitions
	u.Signature = encoding.JsonUnmarshalWith[protocol.KeySignature]{Value: v.Signature, Func: protocol.UnmarshalKeySignatureJSON}
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.PartitionList.Partitions = u.Partitions
	v.Signature = u.Signature.Value

	return nil
}

func (v *SeedCountResponse) UnmarshalJSON(data []byte) error {
	u := struct {
		Count     int64                                             `json:"count,omitempty"`
		Signature encoding.JsonUnmarshalWith[protocol.KeySignature] `json:"signature,omitempty"`
	}{}
	u.Count = v.SeedCount.Count
	u.Signature = encoding.JsonUnmarshalWith[protocol.KeySignature]{Value: v.Signature, Func: protocol.UnmarshalKeySignatureJSON}
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.SeedCount.Count = u.Count
	v.Signature = u.Signature.Value

	return nil
}

func (v *SeedList) UnmarshalJSON(data []byte) error {
	u := struct {
		Addresses encoding.JsonList[string] `json:"addresses,omitempty"`
	}{}
	u.Addresses = v.Addresses
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Addresses = u.Addresses
	return nil
}

func (v *SeedListResponse) UnmarshalJSON(data []byte) error {
	u := struct {
		Addresses encoding.JsonList[string]                         `json:"addresses,omitempty"`
		Signature encoding.JsonUnmarshalWith[protocol.KeySignature] `json:"signature,omitempty"`
	}{}
	u.Addresses = v.SeedList.Addresses
	u.Signature = encoding.JsonUnmarshalWith[protocol.KeySignature]{Value: v.Signature, Func: protocol.UnmarshalKeySignatureJSON}
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.SeedList.Addresses = u.Addresses
	v.Signature = u.Signature.Value

	return nil
}

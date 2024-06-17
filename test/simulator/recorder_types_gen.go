// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

//lint:file-ignore S1001,S1002,S1008,SA4013 generated code

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator/consensus"
)

type recordBlock struct {
	fieldsSet   []bool
	IsLeader    bool                  `json:"isLeader,omitempty" form:"isLeader" query:"isLeader" validate:"required"`
	Index       uint64                `json:"index,omitempty" form:"index" query:"index" validate:"required"`
	Time        time.Time             `json:"time,omitempty" form:"time" query:"time" validate:"required"`
	CommitInfo  json.RawMessage       `json:"commitInfo,omitempty" form:"commitInfo" query:"commitInfo" validate:"required"`
	Evidence    json.RawMessage       `json:"evidence,omitempty" form:"evidence" query:"evidence" validate:"required"`
	Submissions []*messaging.Envelope `json:"submissions,omitempty" form:"submissions" query:"submissions" validate:"required"`
	Changes     []*recordChange       `json:"changes,omitempty" form:"changes" query:"changes" validate:"required"`
	extraData   []byte
}

type recordChange struct {
	fieldsSet []bool
	Key       *record.Key `json:"key,omitempty" form:"key" query:"key" validate:"required"`
	Value     []byte      `json:"value,omitempty" form:"value" query:"value" validate:"required"`
	extraData []byte
}

type recordHeader struct {
	fieldsSet []bool
	Partition *protocol.PartitionInfo `json:"partition,omitempty" form:"partition" query:"partition" validate:"required"`
	Config    *accumulated.NodeInit   `json:"config,omitempty" form:"config" query:"config" validate:"required"`
	NodeID    string                  `json:"nodeID,omitempty" form:"nodeID" query:"nodeID" validate:"required"`
	extraData []byte
}

type recordMessages struct {
	fieldsSet []bool
	Messages  []consensus.Message `json:"messages,omitempty" form:"messages" query:"messages" validate:"required"`
	extraData []byte
}

func (v *recordBlock) Copy() *recordBlock {
	u := new(recordBlock)

	u.IsLeader = v.IsLeader
	u.Index = v.Index
	u.Time = v.Time
	u.CommitInfo = encoding.BytesCopy(v.CommitInfo)
	u.Evidence = encoding.BytesCopy(v.Evidence)
	u.Submissions = make([]*messaging.Envelope, len(v.Submissions))
	for i, v := range v.Submissions {
		v := v
		if v != nil {
			u.Submissions[i] = (v).Copy()
		}
	}
	u.Changes = make([]*recordChange, len(v.Changes))
	for i, v := range v.Changes {
		v := v
		if v != nil {
			u.Changes[i] = (v).Copy()
		}
	}
	if len(v.extraData) > 0 {
		u.extraData = make([]byte, len(v.extraData))
		copy(u.extraData, v.extraData)
	}

	return u
}

func (v *recordBlock) CopyAsInterface() interface{} { return v.Copy() }

func (v *recordChange) Copy() *recordChange {
	u := new(recordChange)

	if v.Key != nil {
		u.Key = (v.Key).Copy()
	}
	u.Value = encoding.BytesCopy(v.Value)
	if len(v.extraData) > 0 {
		u.extraData = make([]byte, len(v.extraData))
		copy(u.extraData, v.extraData)
	}

	return u
}

func (v *recordChange) CopyAsInterface() interface{} { return v.Copy() }

func (v *recordHeader) Copy() *recordHeader {
	u := new(recordHeader)

	if v.Partition != nil {
		u.Partition = (v.Partition).Copy()
	}
	if v.Config != nil {
		u.Config = (v.Config).Copy()
	}
	u.NodeID = v.NodeID
	if len(v.extraData) > 0 {
		u.extraData = make([]byte, len(v.extraData))
		copy(u.extraData, v.extraData)
	}

	return u
}

func (v *recordHeader) CopyAsInterface() interface{} { return v.Copy() }

func (v *recordMessages) Copy() *recordMessages {
	u := new(recordMessages)

	u.Messages = make([]consensus.Message, len(v.Messages))
	for i, v := range v.Messages {
		v := v
		if v != nil {
			u.Messages[i] = consensus.CopyMessage(v)
		}
	}
	if len(v.extraData) > 0 {
		u.extraData = make([]byte, len(v.extraData))
		copy(u.extraData, v.extraData)
	}

	return u
}

func (v *recordMessages) CopyAsInterface() interface{} { return v.Copy() }

func (v *recordBlock) Equal(u *recordBlock) bool {
	if !(v.IsLeader == u.IsLeader) {
		return false
	}
	if !(v.Index == u.Index) {
		return false
	}
	if !((v.Time).Equal(u.Time)) {
		return false
	}
	if !(bytes.Equal(v.CommitInfo, u.CommitInfo)) {
		return false
	}
	if !(bytes.Equal(v.Evidence, u.Evidence)) {
		return false
	}
	if len(v.Submissions) != len(u.Submissions) {
		return false
	}
	for i := range v.Submissions {
		if !((v.Submissions[i]).Equal(u.Submissions[i])) {
			return false
		}
	}
	if len(v.Changes) != len(u.Changes) {
		return false
	}
	for i := range v.Changes {
		if !((v.Changes[i]).Equal(u.Changes[i])) {
			return false
		}
	}

	return true
}

func (v *recordChange) Equal(u *recordChange) bool {
	switch {
	case v.Key == u.Key:
		// equal
	case v.Key == nil || u.Key == nil:
		return false
	case !((v.Key).Equal(u.Key)):
		return false
	}
	if !(bytes.Equal(v.Value, u.Value)) {
		return false
	}

	return true
}

func (v *recordHeader) Equal(u *recordHeader) bool {
	switch {
	case v.Partition == u.Partition:
		// equal
	case v.Partition == nil || u.Partition == nil:
		return false
	case !((v.Partition).Equal(u.Partition)):
		return false
	}
	switch {
	case v.Config == u.Config:
		// equal
	case v.Config == nil || u.Config == nil:
		return false
	case !((v.Config).Equal(u.Config)):
		return false
	}
	if !(v.NodeID == u.NodeID) {
		return false
	}

	return true
}

func (v *recordMessages) Equal(u *recordMessages) bool {
	if len(v.Messages) != len(u.Messages) {
		return false
	}
	for i := range v.Messages {
		if !(consensus.EqualMessage(v.Messages[i], u.Messages[i])) {
			return false
		}
	}

	return true
}

var fieldNames_recordBlock = []string{
	1: "IsLeader",
	2: "Index",
	3: "Time",
	4: "CommitInfo",
	5: "Evidence",
	6: "Submissions",
	7: "Changes",
}

func (v *recordBlock) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(!v.IsLeader) {
		writer.WriteBool(1, v.IsLeader)
	}
	if !(v.Index == 0) {
		writer.WriteUint(2, v.Index)
	}
	if !(v.Time == (time.Time{})) {
		writer.WriteTime(3, v.Time)
	}
	if !(len(v.CommitInfo) == 0) {
		writer.WriteBytes(4, v.CommitInfo)
	}
	if !(len(v.Evidence) == 0) {
		writer.WriteBytes(5, v.Evidence)
	}
	if !(len(v.Submissions) == 0) {
		for _, v := range v.Submissions {
			writer.WriteValue(6, v.MarshalBinary)
		}
	}
	if !(len(v.Changes) == 0) {
		for _, v := range v.Changes {
			writer.WriteValue(7, v.MarshalBinary)
		}
	}

	_, _, err := writer.Reset(fieldNames_recordBlock)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *recordBlock) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field IsLeader is missing")
	} else if !v.IsLeader {
		errs = append(errs, "field IsLeader is not set")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Index is missing")
	} else if v.Index == 0 {
		errs = append(errs, "field Index is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Time is missing")
	} else if v.Time == (time.Time{}) {
		errs = append(errs, "field Time is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field CommitInfo is missing")
	} else if len(v.CommitInfo) == 0 {
		errs = append(errs, "field CommitInfo is not set")
	}
	if len(v.fieldsSet) > 4 && !v.fieldsSet[4] {
		errs = append(errs, "field Evidence is missing")
	} else if len(v.Evidence) == 0 {
		errs = append(errs, "field Evidence is not set")
	}
	if len(v.fieldsSet) > 5 && !v.fieldsSet[5] {
		errs = append(errs, "field Submissions is missing")
	} else if len(v.Submissions) == 0 {
		errs = append(errs, "field Submissions is not set")
	}
	if len(v.fieldsSet) > 6 && !v.fieldsSet[6] {
		errs = append(errs, "field Changes is missing")
	} else if len(v.Changes) == 0 {
		errs = append(errs, "field Changes is not set")
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

var fieldNames_recordChange = []string{
	1: "Key",
	2: "Value",
}

func (v *recordChange) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Key == nil) {
		writer.WriteValue(1, v.Key.MarshalBinary)
	}
	if !(len(v.Value) == 0) {
		writer.WriteBytes(2, v.Value)
	}

	_, _, err := writer.Reset(fieldNames_recordChange)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *recordChange) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Key is missing")
	} else if v.Key == nil {
		errs = append(errs, "field Key is not set")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Value is missing")
	} else if len(v.Value) == 0 {
		errs = append(errs, "field Value is not set")
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

var fieldNames_recordHeader = []string{
	1: "Partition",
	2: "Config",
	3: "NodeID",
}

func (v *recordHeader) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Partition == nil) {
		writer.WriteValue(1, v.Partition.MarshalBinary)
	}
	if !(v.Config == nil) {
		writer.WriteValue(2, v.Config.MarshalBinary)
	}
	if !(len(v.NodeID) == 0) {
		writer.WriteString(3, v.NodeID)
	}

	_, _, err := writer.Reset(fieldNames_recordHeader)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *recordHeader) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Partition is missing")
	} else if v.Partition == nil {
		errs = append(errs, "field Partition is not set")
	}
	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Config is missing")
	} else if v.Config == nil {
		errs = append(errs, "field Config is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field NodeID is missing")
	} else if len(v.NodeID) == 0 {
		errs = append(errs, "field NodeID is not set")
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

var fieldNames_recordMessages = []string{
	1: "Messages",
}

func (v *recordMessages) MarshalBinary() ([]byte, error) {
	if v == nil {
		return []byte{encoding.EmptyObject}, nil
	}

	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Messages) == 0) {
		for _, v := range v.Messages {
			writer.WriteValue(1, v.MarshalBinary)
		}
	}

	_, _, err := writer.Reset(fieldNames_recordMessages)
	if err != nil {
		return nil, encoding.Error{E: err}
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), nil
}

func (v *recordMessages) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 0 && !v.fieldsSet[0] {
		errs = append(errs, "field Messages is missing")
	} else if len(v.Messages) == 0 {
		errs = append(errs, "field Messages is not set")
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

func (v *recordBlock) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *recordBlock) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadBool(1); ok {
		v.IsLeader = x
	}
	if x, ok := reader.ReadUint(2); ok {
		v.Index = x
	}
	if x, ok := reader.ReadTime(3); ok {
		v.Time = x
	}
	if x, ok := reader.ReadBytes(4); ok {
		v.CommitInfo = x
	}
	if x, ok := reader.ReadBytes(5); ok {
		v.Evidence = x
	}
	for {
		if x := new(messaging.Envelope); reader.ReadValue(6, x.UnmarshalBinaryFrom) {
			v.Submissions = append(v.Submissions, x)
		} else {
			break
		}
	}
	for {
		if x := new(recordChange); reader.ReadValue(7, x.UnmarshalBinaryFrom) {
			v.Changes = append(v.Changes, x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_recordBlock)
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

func (v *recordChange) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *recordChange) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x := new(record.Key); reader.ReadValue(1, x.UnmarshalBinaryFrom) {
		v.Key = x
	}
	if x, ok := reader.ReadBytes(2); ok {
		v.Value = x
	}

	seen, err := reader.Reset(fieldNames_recordChange)
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

func (v *recordHeader) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *recordHeader) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x := new(protocol.PartitionInfo); reader.ReadValue(1, x.UnmarshalBinaryFrom) {
		v.Partition = x
	}
	if x := new(accumulated.NodeInit); reader.ReadValue(2, x.UnmarshalBinaryFrom) {
		v.Config = x
	}
	if x, ok := reader.ReadString(3); ok {
		v.NodeID = x
	}

	seen, err := reader.Reset(fieldNames_recordHeader)
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

func (v *recordMessages) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *recordMessages) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	for {
		ok := reader.ReadValue(1, func(r io.Reader) error {
			x, err := consensus.UnmarshalMessageFrom(r)
			if err == nil {
				v.Messages = append(v.Messages, x)
			}
			return err
		})
		if !ok {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_recordMessages)
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

func (v *recordBlock) MarshalJSON() ([]byte, error) {
	u := struct {
		IsLeader    bool                                   `json:"isLeader,omitempty"`
		Index       uint64                                 `json:"index,omitempty"`
		Time        time.Time                              `json:"time,omitempty"`
		CommitInfo  json.RawMessage                        `json:"commitInfo,omitempty"`
		Evidence    json.RawMessage                        `json:"evidence,omitempty"`
		Submissions encoding.JsonList[*messaging.Envelope] `json:"submissions,omitempty"`
		Changes     encoding.JsonList[*recordChange]       `json:"changes,omitempty"`
		ExtraData   *string                                `json:"$epilogue,omitempty"`
	}{}
	if !(!v.IsLeader) {
		u.IsLeader = v.IsLeader
	}
	if !(v.Index == 0) {
		u.Index = v.Index
	}
	if !(v.Time == (time.Time{})) {
		u.Time = v.Time
	}
	if !(len(v.CommitInfo) == 0) {
		u.CommitInfo = v.CommitInfo
	}
	if !(len(v.Evidence) == 0) {
		u.Evidence = v.Evidence
	}
	if !(len(v.Submissions) == 0) {
		u.Submissions = v.Submissions
	}
	if !(len(v.Changes) == 0) {
		u.Changes = v.Changes
	}
	u.ExtraData = encoding.BytesToJSON(v.extraData)
	return json.Marshal(&u)
}

func (v *recordChange) MarshalJSON() ([]byte, error) {
	u := struct {
		Key       *record.Key `json:"key,omitempty"`
		Value     *string     `json:"value,omitempty"`
		ExtraData *string     `json:"$epilogue,omitempty"`
	}{}
	if !(v.Key == nil) {
		u.Key = v.Key
	}
	if !(len(v.Value) == 0) {
		u.Value = encoding.BytesToJSON(v.Value)
	}
	u.ExtraData = encoding.BytesToJSON(v.extraData)
	return json.Marshal(&u)
}

func (v *recordMessages) MarshalJSON() ([]byte, error) {
	u := struct {
		Messages  *encoding.JsonUnmarshalListWith[consensus.Message] `json:"messages,omitempty"`
		ExtraData *string                                            `json:"$epilogue,omitempty"`
	}{}
	if !(len(v.Messages) == 0) {
		u.Messages = &encoding.JsonUnmarshalListWith[consensus.Message]{Value: v.Messages, Func: consensus.UnmarshalMessageJSON}
	}
	u.ExtraData = encoding.BytesToJSON(v.extraData)
	return json.Marshal(&u)
}

func (v *recordBlock) UnmarshalJSON(data []byte) error {
	u := struct {
		IsLeader    bool                                   `json:"isLeader,omitempty"`
		Index       uint64                                 `json:"index,omitempty"`
		Time        time.Time                              `json:"time,omitempty"`
		CommitInfo  json.RawMessage                        `json:"commitInfo,omitempty"`
		Evidence    json.RawMessage                        `json:"evidence,omitempty"`
		Submissions encoding.JsonList[*messaging.Envelope] `json:"submissions,omitempty"`
		Changes     encoding.JsonList[*recordChange]       `json:"changes,omitempty"`
		ExtraData   *string                                `json:"$epilogue,omitempty"`
	}{}
	u.IsLeader = v.IsLeader
	u.Index = v.Index
	u.Time = v.Time
	u.CommitInfo = v.CommitInfo
	u.Evidence = v.Evidence
	u.Submissions = v.Submissions
	u.Changes = v.Changes
	err := json.Unmarshal(data, &u)
	if err != nil {
		return err
	}
	v.IsLeader = u.IsLeader
	v.Index = u.Index
	v.Time = u.Time
	v.CommitInfo = u.CommitInfo
	v.Evidence = u.Evidence
	v.Submissions = u.Submissions
	v.Changes = u.Changes
	v.extraData, err = encoding.BytesFromJSON(u.ExtraData)
	if err != nil {
		return err
	}
	return nil
}

func (v *recordChange) UnmarshalJSON(data []byte) error {
	u := struct {
		Key       *record.Key `json:"key,omitempty"`
		Value     *string     `json:"value,omitempty"`
		ExtraData *string     `json:"$epilogue,omitempty"`
	}{}
	u.Key = v.Key
	u.Value = encoding.BytesToJSON(v.Value)
	err := json.Unmarshal(data, &u)
	if err != nil {
		return err
	}
	v.Key = u.Key
	if x, err := encoding.BytesFromJSON(u.Value); err != nil {
		return fmt.Errorf("error decoding Value: %w", err)
	} else {
		v.Value = x
	}
	v.extraData, err = encoding.BytesFromJSON(u.ExtraData)
	if err != nil {
		return err
	}
	return nil
}

func (v *recordMessages) UnmarshalJSON(data []byte) error {
	u := struct {
		Messages  *encoding.JsonUnmarshalListWith[consensus.Message] `json:"messages,omitempty"`
		ExtraData *string                                            `json:"$epilogue,omitempty"`
	}{}
	u.Messages = &encoding.JsonUnmarshalListWith[consensus.Message]{Value: v.Messages, Func: consensus.UnmarshalMessageJSON}
	err := json.Unmarshal(data, &u)
	if err != nil {
		return err
	}
	if u.Messages != nil && u.Messages.Value != nil {
		v.Messages = make([]consensus.Message, len(u.Messages.Value))
		for i, x := range u.Messages.Value {
			v.Messages[i] = x
		}
	}
	v.extraData, err = encoding.BytesFromJSON(u.ExtraData)
	if err != nil {
		return err
	}
	return nil
}

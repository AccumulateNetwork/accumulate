package api

// GENERATED BY go run ./tools/cmd/gen-types. DO NOT EDIT.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	errors2 "gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

type AccountRecord struct {
	fieldsSet []bool
	Account   protocol.Account `json:"account,omitempty" form:"account" query:"account" validate:"required"`
	Chains    []*ChainState    `json:"chains,omitempty" form:"chains" query:"chains" validate:"required"`
	Proof     *Receipt         `json:"proof,omitempty" form:"proof" query:"proof" validate:"required"`
	extraData []byte
}

type ChainState struct {
	fieldsSet []bool
	Name      string             `json:"name,omitempty" form:"name" query:"name" validate:"required"`
	Type      protocol.ChainType `json:"type,omitempty" form:"type" query:"type" validate:"required"`
	Height    uint64             `json:"height,omitempty" form:"height" query:"height" validate:"required"`
	Roots     [][]byte           `json:"roots,omitempty" form:"roots" query:"roots" validate:"required"`
	extraData []byte
}

type NetworkMetrics struct {
	fieldsSet []bool
	TPS       float64 `json:"tPS,omitempty" form:"tPS" query:"tPS" validate:"required"`
	extraData []byte
}

type NodeDescription struct {
	Partition config.Network `json:"partition,omitempty" form:"partition" query:"partition" validate:"required"`
	extraData []byte
}

type NodeMetrics struct {
	fieldsSet []bool
	TPS       float64 `json:"tPS,omitempty" form:"tPS" query:"tPS" validate:"required"`
	extraData []byte
}

type NodeStatus struct {
	fieldsSet []bool
	Ok        bool `json:"ok,omitempty" form:"ok" query:"ok" validate:"required"`
	extraData []byte
}

type NodeVersion struct {
	fieldsSet      []bool
	Version        string `json:"version,omitempty" form:"version" query:"version" validate:"required"`
	Commit         string `json:"commit,omitempty" form:"commit" query:"commit" validate:"required"`
	VersionIsKnown bool   `json:"versionIsKnown,omitempty" form:"versionIsKnown" query:"versionIsKnown" validate:"required"`
	IsTestNet      bool   `json:"isTestNet,omitempty" form:"isTestNet" query:"isTestNet" validate:"required"`
	extraData      []byte
}

type QuerySetOptions struct {
	fieldsSet []bool
	// Expand requests an expanded response.
	Expand bool `json:"expand,omitempty" form:"expand" query:"expand"`
	// Start is the starting index.
	Start bool `json:"start,omitempty" form:"start" query:"start"`
	// Count is the number of requested results.
	Count     bool `json:"count,omitempty" form:"count" query:"count"`
	extraData []byte
}

type QueryStateOptions struct {
	fieldsSet []bool
	// Prove requests a proof.
	Prove bool `json:"prove,omitempty" form:"prove" query:"prove"`
	// Expand requests an expanded response.
	Expand bool `json:"expand,omitempty" form:"expand" query:"expand"`
	// Wait blocks until the record is available.
	Wait time.Duration `json:"wait,omitempty" form:"wait" query:"wait"`
	// Delivered ignores incomplete transactions.
	Delivered bool `json:"delivered,omitempty" form:"delivered" query:"delivered"`
	extraData []byte
}

type Receipt struct {
	fieldsSet      []bool
	LocalBlock     uint64          `json:"localBlock,omitempty" form:"localBlock" query:"localBlock" validate:"required"`
	DirectoryBlock uint64          `json:"directoryBlock,omitempty" form:"directoryBlock" query:"directoryBlock" validate:"required"`
	Proof          managed.Receipt `json:"proof,omitempty" form:"proof" query:"proof" validate:"required"`
	Error          *errors2.Error  `json:"error,omitempty" form:"error" query:"error" validate:"required"`
	extraData      []byte
}

type SearchOptions struct {
	fieldsSet []bool
	// Kind is the kind of record to search for.
	Kind      string `json:"kind,omitempty" form:"kind" query:"kind" validate:"required"`
	extraData []byte
}

type Submission struct {
	fieldsSet         []bool
	TransactionHashes [][32]byte                    `json:"transactionHashes,omitempty" form:"transactionHashes" query:"transactionHashes" validate:"required"`
	SignatureHashes   [][32]byte                    `json:"signatureHashes,omitempty" form:"signatureHashes" query:"signatureHashes" validate:"required"`
	Status            []*protocol.TransactionStatus `json:"status,omitempty" form:"status" query:"status" validate:"required"`
	extraData         []byte
}

type SubmitOptions struct {
	fieldsSet []bool
	Mode      SubmitMode `json:"mode,omitempty" form:"mode" query:"mode" validate:"required"`
	extraData []byte
}

func (*AccountRecord) Type() RecordType { return RecordTypeAccount }

func (v *AccountRecord) Copy() *AccountRecord {
	u := new(AccountRecord)

	if v.Account != nil {
		u.Account = (v.Account).CopyAsInterface().(protocol.Account)
	}
	u.Chains = make([]*ChainState, len(v.Chains))
	for i, v := range v.Chains {
		if v != nil {
			u.Chains[i] = (v).Copy()
		}
	}
	if v.Proof != nil {
		u.Proof = (v.Proof).Copy()
	}

	return u
}

func (v *AccountRecord) CopyAsInterface() interface{} { return v.Copy() }

func (v *ChainState) Copy() *ChainState {
	u := new(ChainState)

	u.Name = v.Name
	u.Type = v.Type
	u.Height = v.Height
	u.Roots = make([][]byte, len(v.Roots))
	for i, v := range v.Roots {
		u.Roots[i] = encoding.BytesCopy(v)
	}

	return u
}

func (v *ChainState) CopyAsInterface() interface{} { return v.Copy() }

func (v *NetworkMetrics) Copy() *NetworkMetrics {
	u := new(NetworkMetrics)

	u.TPS = v.TPS

	return u
}

func (v *NetworkMetrics) CopyAsInterface() interface{} { return v.Copy() }

func (v *NodeMetrics) Copy() *NodeMetrics {
	u := new(NodeMetrics)

	u.TPS = v.TPS

	return u
}

func (v *NodeMetrics) CopyAsInterface() interface{} { return v.Copy() }

func (v *NodeStatus) Copy() *NodeStatus {
	u := new(NodeStatus)

	u.Ok = v.Ok

	return u
}

func (v *NodeStatus) CopyAsInterface() interface{} { return v.Copy() }

func (v *NodeVersion) Copy() *NodeVersion {
	u := new(NodeVersion)

	u.Version = v.Version
	u.Commit = v.Commit
	u.VersionIsKnown = v.VersionIsKnown
	u.IsTestNet = v.IsTestNet

	return u
}

func (v *NodeVersion) CopyAsInterface() interface{} { return v.Copy() }

func (v *QuerySetOptions) Copy() *QuerySetOptions {
	u := new(QuerySetOptions)

	u.Expand = v.Expand
	u.Start = v.Start
	u.Count = v.Count

	return u
}

func (v *QuerySetOptions) CopyAsInterface() interface{} { return v.Copy() }

func (v *QueryStateOptions) Copy() *QueryStateOptions {
	u := new(QueryStateOptions)

	u.Prove = v.Prove
	u.Expand = v.Expand
	u.Wait = v.Wait
	u.Delivered = v.Delivered

	return u
}

func (v *QueryStateOptions) CopyAsInterface() interface{} { return v.Copy() }

func (v *Receipt) Copy() *Receipt {
	u := new(Receipt)

	u.LocalBlock = v.LocalBlock
	u.DirectoryBlock = v.DirectoryBlock
	u.Proof = *(&v.Proof).Copy()
	if v.Error != nil {
		u.Error = (v.Error).Copy()
	}

	return u
}

func (v *Receipt) CopyAsInterface() interface{} { return v.Copy() }

func (v *SearchOptions) Copy() *SearchOptions {
	u := new(SearchOptions)

	u.Kind = v.Kind

	return u
}

func (v *SearchOptions) CopyAsInterface() interface{} { return v.Copy() }

func (v *Submission) Copy() *Submission {
	u := new(Submission)

	u.TransactionHashes = make([][32]byte, len(v.TransactionHashes))
	for i, v := range v.TransactionHashes {
		u.TransactionHashes[i] = v
	}
	u.SignatureHashes = make([][32]byte, len(v.SignatureHashes))
	for i, v := range v.SignatureHashes {
		u.SignatureHashes[i] = v
	}
	u.Status = make([]*protocol.TransactionStatus, len(v.Status))
	for i, v := range v.Status {
		if v != nil {
			u.Status[i] = (v).Copy()
		}
	}

	return u
}

func (v *Submission) CopyAsInterface() interface{} { return v.Copy() }

func (v *SubmitOptions) Copy() *SubmitOptions {
	u := new(SubmitOptions)

	u.Mode = v.Mode

	return u
}

func (v *SubmitOptions) CopyAsInterface() interface{} { return v.Copy() }

func (v *AccountRecord) Equal(u *AccountRecord) bool {
	if !(protocol.EqualAccount(v.Account, u.Account)) {
		return false
	}
	if len(v.Chains) != len(u.Chains) {
		return false
	}
	for i := range v.Chains {
		if !((v.Chains[i]).Equal(u.Chains[i])) {
			return false
		}
	}
	switch {
	case v.Proof == u.Proof:
		// equal
	case v.Proof == nil || u.Proof == nil:
		return false
	case !((v.Proof).Equal(u.Proof)):
		return false
	}

	return true
}

func (v *ChainState) Equal(u *ChainState) bool {
	if !(v.Name == u.Name) {
		return false
	}
	if !(v.Type == u.Type) {
		return false
	}
	if !(v.Height == u.Height) {
		return false
	}
	if len(v.Roots) != len(u.Roots) {
		return false
	}
	for i := range v.Roots {
		if !(bytes.Equal(v.Roots[i], u.Roots[i])) {
			return false
		}
	}

	return true
}

func (v *NetworkMetrics) Equal(u *NetworkMetrics) bool {
	if !(v.TPS == u.TPS) {
		return false
	}

	return true
}

func (v *NodeMetrics) Equal(u *NodeMetrics) bool {
	if !(v.TPS == u.TPS) {
		return false
	}

	return true
}

func (v *NodeStatus) Equal(u *NodeStatus) bool {
	if !(v.Ok == u.Ok) {
		return false
	}

	return true
}

func (v *NodeVersion) Equal(u *NodeVersion) bool {
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

func (v *QuerySetOptions) Equal(u *QuerySetOptions) bool {
	if !(v.Expand == u.Expand) {
		return false
	}
	if !(v.Start == u.Start) {
		return false
	}
	if !(v.Count == u.Count) {
		return false
	}

	return true
}

func (v *QueryStateOptions) Equal(u *QueryStateOptions) bool {
	if !(v.Prove == u.Prove) {
		return false
	}
	if !(v.Expand == u.Expand) {
		return false
	}
	if !(v.Wait == u.Wait) {
		return false
	}
	if !(v.Delivered == u.Delivered) {
		return false
	}

	return true
}

func (v *Receipt) Equal(u *Receipt) bool {
	if !(v.LocalBlock == u.LocalBlock) {
		return false
	}
	if !(v.DirectoryBlock == u.DirectoryBlock) {
		return false
	}
	if !((&v.Proof).Equal(&u.Proof)) {
		return false
	}
	switch {
	case v.Error == u.Error:
		// equal
	case v.Error == nil || u.Error == nil:
		return false
	case !((v.Error).Equal(u.Error)):
		return false
	}

	return true
}

func (v *SearchOptions) Equal(u *SearchOptions) bool {
	if !(v.Kind == u.Kind) {
		return false
	}

	return true
}

func (v *Submission) Equal(u *Submission) bool {
	if len(v.TransactionHashes) != len(u.TransactionHashes) {
		return false
	}
	for i := range v.TransactionHashes {
		if !(v.TransactionHashes[i] == u.TransactionHashes[i]) {
			return false
		}
	}
	if len(v.SignatureHashes) != len(u.SignatureHashes) {
		return false
	}
	for i := range v.SignatureHashes {
		if !(v.SignatureHashes[i] == u.SignatureHashes[i]) {
			return false
		}
	}
	if len(v.Status) != len(u.Status) {
		return false
	}
	for i := range v.Status {
		if !((v.Status[i]).Equal(u.Status[i])) {
			return false
		}
	}

	return true
}

func (v *SubmitOptions) Equal(u *SubmitOptions) bool {
	if !(v.Mode == u.Mode) {
		return false
	}

	return true
}

var fieldNames_AccountRecord = []string{
	1: "Type",
	2: "Account",
	3: "Chains",
	4: "Proof",
}

func (v *AccountRecord) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	writer.WriteEnum(1, v.Type())
	if !(v.Account == nil) {
		writer.WriteValue(2, v.Account)
	}
	if !(len(v.Chains) == 0) {
		for _, v := range v.Chains {
			writer.WriteValue(3, v)
		}
	}
	if !(v.Proof == nil) {
		writer.WriteValue(4, v.Proof)
	}

	_, _, err := writer.Reset(fieldNames_AccountRecord)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *AccountRecord) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Type is missing")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Account is missing")
	} else if v.Account == nil {
		errs = append(errs, "field Account is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field Chains is missing")
	} else if len(v.Chains) == 0 {
		errs = append(errs, "field Chains is not set")
	}
	if len(v.fieldsSet) > 4 && !v.fieldsSet[4] {
		errs = append(errs, "field Proof is missing")
	} else if v.Proof == nil {
		errs = append(errs, "field Proof is not set")
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

var fieldNames_ChainState = []string{
	1: "Name",
	2: "Type",
	3: "Height",
	4: "Roots",
}

func (v *ChainState) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Name) == 0) {
		writer.WriteString(1, v.Name)
	}
	if !(v.Type == 0) {
		writer.WriteEnum(2, v.Type)
	}
	if !(v.Height == 0) {
		writer.WriteUint(3, v.Height)
	}
	if !(len(v.Roots) == 0) {
		for _, v := range v.Roots {
			writer.WriteBytes(4, v)
		}
	}

	_, _, err := writer.Reset(fieldNames_ChainState)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *ChainState) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Name is missing")
	} else if len(v.Name) == 0 {
		errs = append(errs, "field Name is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field Type is missing")
	} else if v.Type == 0 {
		errs = append(errs, "field Type is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field Height is missing")
	} else if v.Height == 0 {
		errs = append(errs, "field Height is not set")
	}
	if len(v.fieldsSet) > 4 && !v.fieldsSet[4] {
		errs = append(errs, "field Roots is missing")
	} else if len(v.Roots) == 0 {
		errs = append(errs, "field Roots is not set")
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

var fieldNames_NetworkMetrics = []string{
	1: "TPS",
}

func (v *NetworkMetrics) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.TPS == 0) {
		writer.WriteFloat(1, v.TPS)
	}

	_, _, err := writer.Reset(fieldNames_NetworkMetrics)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *NetworkMetrics) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field TPS is missing")
	} else if v.TPS == 0 {
		errs = append(errs, "field TPS is not set")
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

var fieldNames_NodeMetrics = []string{
	1: "TPS",
}

func (v *NodeMetrics) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.TPS == 0) {
		writer.WriteFloat(1, v.TPS)
	}

	_, _, err := writer.Reset(fieldNames_NodeMetrics)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *NodeMetrics) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field TPS is missing")
	} else if v.TPS == 0 {
		errs = append(errs, "field TPS is not set")
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

var fieldNames_NodeStatus = []string{
	1: "Ok",
}

func (v *NodeStatus) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(!v.Ok) {
		writer.WriteBool(1, v.Ok)
	}

	_, _, err := writer.Reset(fieldNames_NodeStatus)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *NodeStatus) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Ok is missing")
	} else if !v.Ok {
		errs = append(errs, "field Ok is not set")
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

var fieldNames_NodeVersion = []string{
	1: "Version",
	2: "Commit",
	3: "VersionIsKnown",
	4: "IsTestNet",
}

func (v *NodeVersion) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Version) == 0) {
		writer.WriteString(1, v.Version)
	}
	if !(len(v.Commit) == 0) {
		writer.WriteString(2, v.Commit)
	}
	if !(!v.VersionIsKnown) {
		writer.WriteBool(3, v.VersionIsKnown)
	}
	if !(!v.IsTestNet) {
		writer.WriteBool(4, v.IsTestNet)
	}

	_, _, err := writer.Reset(fieldNames_NodeVersion)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *NodeVersion) IsValid() error {
	var errs []string

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

var fieldNames_QuerySetOptions = []string{
	1: "Expand",
	2: "Start",
	3: "Count",
}

func (v *QuerySetOptions) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(!v.Expand) {
		writer.WriteBool(1, v.Expand)
	}
	if !(!v.Start) {
		writer.WriteBool(2, v.Start)
	}
	if !(!v.Count) {
		writer.WriteBool(3, v.Count)
	}

	_, _, err := writer.Reset(fieldNames_QuerySetOptions)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *QuerySetOptions) IsValid() error {
	var errs []string

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

var fieldNames_QueryStateOptions = []string{
	1: "Prove",
	2: "Expand",
	3: "Wait",
	4: "Delivered",
}

func (v *QueryStateOptions) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(!v.Prove) {
		writer.WriteBool(1, v.Prove)
	}
	if !(!v.Expand) {
		writer.WriteBool(2, v.Expand)
	}
	if !(v.Wait == 0) {
		writer.WriteDuration(3, v.Wait)
	}
	if !(!v.Delivered) {
		writer.WriteBool(4, v.Delivered)
	}

	_, _, err := writer.Reset(fieldNames_QueryStateOptions)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *QueryStateOptions) IsValid() error {
	var errs []string

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errors.New(errs[0])
	default:
		return errors.New(strings.Join(errs, "; "))
	}
}

var fieldNames_Receipt = []string{
	1: "LocalBlock",
	2: "DirectoryBlock",
	3: "Proof",
	4: "Error",
}

func (v *Receipt) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.LocalBlock == 0) {
		writer.WriteUint(1, v.LocalBlock)
	}
	if !(v.DirectoryBlock == 0) {
		writer.WriteUint(2, v.DirectoryBlock)
	}
	if !((v.Proof).Equal(new(managed.Receipt))) {
		writer.WriteValue(3, &v.Proof)
	}
	if !(v.Error == nil) {
		writer.WriteValue(4, v.Error)
	}

	_, _, err := writer.Reset(fieldNames_Receipt)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *Receipt) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field LocalBlock is missing")
	} else if v.LocalBlock == 0 {
		errs = append(errs, "field LocalBlock is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field DirectoryBlock is missing")
	} else if v.DirectoryBlock == 0 {
		errs = append(errs, "field DirectoryBlock is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field Proof is missing")
	} else if (v.Proof).Equal(new(managed.Receipt)) {
		errs = append(errs, "field Proof is not set")
	}
	if len(v.fieldsSet) > 4 && !v.fieldsSet[4] {
		errs = append(errs, "field Error is missing")
	} else if v.Error == nil {
		errs = append(errs, "field Error is not set")
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

var fieldNames_SearchOptions = []string{
	1: "Kind",
}

func (v *SearchOptions) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.Kind) == 0) {
		writer.WriteString(1, v.Kind)
	}

	_, _, err := writer.Reset(fieldNames_SearchOptions)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *SearchOptions) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Kind is missing")
	} else if len(v.Kind) == 0 {
		errs = append(errs, "field Kind is not set")
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

var fieldNames_Submission = []string{
	1: "TransactionHashes",
	2: "SignatureHashes",
	3: "Status",
}

func (v *Submission) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(len(v.TransactionHashes) == 0) {
		for _, v := range v.TransactionHashes {
			writer.WriteHash(1, &v)
		}
	}
	if !(len(v.SignatureHashes) == 0) {
		for _, v := range v.SignatureHashes {
			writer.WriteHash(2, &v)
		}
	}
	if !(len(v.Status) == 0) {
		for _, v := range v.Status {
			writer.WriteValue(3, v)
		}
	}

	_, _, err := writer.Reset(fieldNames_Submission)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *Submission) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field TransactionHashes is missing")
	} else if len(v.TransactionHashes) == 0 {
		errs = append(errs, "field TransactionHashes is not set")
	}
	if len(v.fieldsSet) > 2 && !v.fieldsSet[2] {
		errs = append(errs, "field SignatureHashes is missing")
	} else if len(v.SignatureHashes) == 0 {
		errs = append(errs, "field SignatureHashes is not set")
	}
	if len(v.fieldsSet) > 3 && !v.fieldsSet[3] {
		errs = append(errs, "field Status is missing")
	} else if len(v.Status) == 0 {
		errs = append(errs, "field Status is not set")
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

var fieldNames_SubmitOptions = []string{
	1: "Mode",
}

func (v *SubmitOptions) MarshalBinary() ([]byte, error) {
	buffer := new(bytes.Buffer)
	writer := encoding.NewWriter(buffer)

	if !(v.Mode == 0) {
		writer.WriteEnum(1, v.Mode)
	}

	_, _, err := writer.Reset(fieldNames_SubmitOptions)
	if err != nil {
		return nil, err
	}
	buffer.Write(v.extraData)
	return buffer.Bytes(), err
}

func (v *SubmitOptions) IsValid() error {
	var errs []string

	if len(v.fieldsSet) > 1 && !v.fieldsSet[1] {
		errs = append(errs, "field Mode is missing")
	} else if v.Mode == 0 {
		errs = append(errs, "field Mode is not set")
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

func (v *AccountRecord) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *AccountRecord) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	var vType RecordType
	if x := new(RecordType); reader.ReadEnum(1, x) {
		vType = *x
	}
	if !(v.Type() == vType) {
		return fmt.Errorf("field Type: not equal: want %v, got %v", v.Type(), vType)
	}
	reader.ReadValue(2, func(b []byte) error {
		x, err := protocol.UnmarshalAccount(b)
		if err == nil {
			v.Account = x
		}
		return err
	})
	for {
		if x := new(ChainState); reader.ReadValue(3, x.UnmarshalBinary) {
			v.Chains = append(v.Chains, x)
		} else {
			break
		}
	}
	if x := new(Receipt); reader.ReadValue(4, x.UnmarshalBinary) {
		v.Proof = x
	}

	seen, err := reader.Reset(fieldNames_AccountRecord)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *ChainState) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *ChainState) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadString(1); ok {
		v.Name = x
	}
	if x := new(protocol.ChainType); reader.ReadEnum(2, x) {
		v.Type = *x
	}
	if x, ok := reader.ReadUint(3); ok {
		v.Height = x
	}
	for {
		if x, ok := reader.ReadBytes(4); ok {
			v.Roots = append(v.Roots, x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_ChainState)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *NetworkMetrics) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *NetworkMetrics) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadFloat(1); ok {
		v.TPS = x
	}

	seen, err := reader.Reset(fieldNames_NetworkMetrics)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *NodeMetrics) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *NodeMetrics) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadFloat(1); ok {
		v.TPS = x
	}

	seen, err := reader.Reset(fieldNames_NodeMetrics)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *NodeStatus) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *NodeStatus) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadBool(1); ok {
		v.Ok = x
	}

	seen, err := reader.Reset(fieldNames_NodeStatus)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *NodeVersion) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *NodeVersion) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadString(1); ok {
		v.Version = x
	}
	if x, ok := reader.ReadString(2); ok {
		v.Commit = x
	}
	if x, ok := reader.ReadBool(3); ok {
		v.VersionIsKnown = x
	}
	if x, ok := reader.ReadBool(4); ok {
		v.IsTestNet = x
	}

	seen, err := reader.Reset(fieldNames_NodeVersion)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *QuerySetOptions) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *QuerySetOptions) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadBool(1); ok {
		v.Expand = x
	}
	if x, ok := reader.ReadBool(2); ok {
		v.Start = x
	}
	if x, ok := reader.ReadBool(3); ok {
		v.Count = x
	}

	seen, err := reader.Reset(fieldNames_QuerySetOptions)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *QueryStateOptions) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *QueryStateOptions) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadBool(1); ok {
		v.Prove = x
	}
	if x, ok := reader.ReadBool(2); ok {
		v.Expand = x
	}
	if x, ok := reader.ReadDuration(3); ok {
		v.Wait = x
	}
	if x, ok := reader.ReadBool(4); ok {
		v.Delivered = x
	}

	seen, err := reader.Reset(fieldNames_QueryStateOptions)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *Receipt) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *Receipt) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadUint(1); ok {
		v.LocalBlock = x
	}
	if x, ok := reader.ReadUint(2); ok {
		v.DirectoryBlock = x
	}
	if x := new(managed.Receipt); reader.ReadValue(3, x.UnmarshalBinary) {
		v.Proof = *x
	}
	if x := new(errors2.Error); reader.ReadValue(4, x.UnmarshalBinary) {
		v.Error = x
	}

	seen, err := reader.Reset(fieldNames_Receipt)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *SearchOptions) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *SearchOptions) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x, ok := reader.ReadString(1); ok {
		v.Kind = x
	}

	seen, err := reader.Reset(fieldNames_SearchOptions)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *Submission) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *Submission) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	for {
		if x, ok := reader.ReadHash(1); ok {
			v.TransactionHashes = append(v.TransactionHashes, *x)
		} else {
			break
		}
	}
	for {
		if x, ok := reader.ReadHash(2); ok {
			v.SignatureHashes = append(v.SignatureHashes, *x)
		} else {
			break
		}
	}
	for {
		if x := new(protocol.TransactionStatus); reader.ReadValue(3, x.UnmarshalBinary) {
			v.Status = append(v.Status, x)
		} else {
			break
		}
	}

	seen, err := reader.Reset(fieldNames_Submission)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *SubmitOptions) UnmarshalBinary(data []byte) error {
	return v.UnmarshalBinaryFrom(bytes.NewReader(data))
}

func (v *SubmitOptions) UnmarshalBinaryFrom(rd io.Reader) error {
	reader := encoding.NewReader(rd)

	if x := new(SubmitMode); reader.ReadEnum(1, x) {
		v.Mode = *x
	}

	seen, err := reader.Reset(fieldNames_SubmitOptions)
	if err != nil {
		return err
	}
	v.fieldsSet = seen
	v.extraData, err = reader.ReadAll()
	return err
}

func (v *AccountRecord) MarshalJSON() ([]byte, error) {
	u := struct {
		Type    RecordType                                   `json:"type"`
		Account encoding.JsonUnmarshalWith[protocol.Account] `json:"account,omitempty"`
		Chains  encoding.JsonList[*ChainState]               `json:"chains,omitempty"`
		Proof   *Receipt                                     `json:"proof,omitempty"`
	}{}
	u.Type = v.Type()
	u.Account = encoding.JsonUnmarshalWith[protocol.Account]{Value: v.Account, Func: protocol.UnmarshalAccountJSON}
	u.Chains = v.Chains
	u.Proof = v.Proof
	return json.Marshal(&u)
}

func (v *ChainState) MarshalJSON() ([]byte, error) {
	u := struct {
		Name   string                     `json:"name,omitempty"`
		Type   protocol.ChainType         `json:"type,omitempty"`
		Height uint64                     `json:"height,omitempty"`
		Count  uint64                     `json:"count,omitempty"`
		Roots  encoding.JsonList[*string] `json:"roots,omitempty"`
	}{}
	u.Name = v.Name
	u.Type = v.Type
	u.Height = v.Height
	u.Count = v.Height
	u.Roots = make(encoding.JsonList[*string], len(v.Roots))
	for i, x := range v.Roots {
		u.Roots[i] = encoding.BytesToJSON(x)
	}
	return json.Marshal(&u)
}

func (v *QueryStateOptions) MarshalJSON() ([]byte, error) {
	u := struct {
		Prove     bool        `json:"prove,omitempty"`
		Expand    bool        `json:"expand,omitempty"`
		Wait      interface{} `json:"wait,omitempty"`
		Delivered bool        `json:"delivered,omitempty"`
	}{}
	u.Prove = v.Prove
	u.Expand = v.Expand
	u.Wait = encoding.DurationToJSON(v.Wait)
	u.Delivered = v.Delivered
	return json.Marshal(&u)
}

func (v *Submission) MarshalJSON() ([]byte, error) {
	u := struct {
		TransactionHashes encoding.JsonList[string]                      `json:"transactionHashes,omitempty"`
		SignatureHashes   encoding.JsonList[string]                      `json:"signatureHashes,omitempty"`
		Status            encoding.JsonList[*protocol.TransactionStatus] `json:"status,omitempty"`
	}{}
	u.TransactionHashes = make(encoding.JsonList[string], len(v.TransactionHashes))
	for i, x := range v.TransactionHashes {
		u.TransactionHashes[i] = encoding.ChainToJSON(x)
	}
	u.SignatureHashes = make(encoding.JsonList[string], len(v.SignatureHashes))
	for i, x := range v.SignatureHashes {
		u.SignatureHashes[i] = encoding.ChainToJSON(x)
	}
	u.Status = v.Status
	return json.Marshal(&u)
}

func (v *AccountRecord) UnmarshalJSON(data []byte) error {
	u := struct {
		Type    RecordType                                   `json:"type"`
		Account encoding.JsonUnmarshalWith[protocol.Account] `json:"account,omitempty"`
		Chains  encoding.JsonList[*ChainState]               `json:"chains,omitempty"`
		Proof   *Receipt                                     `json:"proof,omitempty"`
	}{}
	u.Type = v.Type()
	u.Account = encoding.JsonUnmarshalWith[protocol.Account]{Value: v.Account, Func: protocol.UnmarshalAccountJSON}
	u.Chains = v.Chains
	u.Proof = v.Proof
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	if !(v.Type() == u.Type) {
		return fmt.Errorf("field Type: not equal: want %v, got %v", v.Type(), u.Type)
	}
	v.Account = u.Account.Value

	v.Chains = u.Chains
	v.Proof = u.Proof
	return nil
}

func (v *ChainState) UnmarshalJSON(data []byte) error {
	u := struct {
		Name   string                     `json:"name,omitempty"`
		Type   protocol.ChainType         `json:"type,omitempty"`
		Height uint64                     `json:"height,omitempty"`
		Count  uint64                     `json:"count,omitempty"`
		Roots  encoding.JsonList[*string] `json:"roots,omitempty"`
	}{}
	u.Name = v.Name
	u.Type = v.Type
	u.Height = v.Height
	u.Count = v.Height
	u.Roots = make(encoding.JsonList[*string], len(v.Roots))
	for i, x := range v.Roots {
		u.Roots[i] = encoding.BytesToJSON(x)
	}
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Name = u.Name
	v.Type = u.Type
	if !(u.Height == 0) {
		v.Height = u.Height
	} else {
		v.Height = u.Count
	}
	v.Roots = make([][]byte, len(u.Roots))
	for i, x := range u.Roots {
		if x, err := encoding.BytesFromJSON(x); err != nil {
			return fmt.Errorf("error decoding Roots: %w", err)
		} else {
			v.Roots[i] = x
		}
	}
	return nil
}

func (v *QueryStateOptions) UnmarshalJSON(data []byte) error {
	u := struct {
		Prove     bool        `json:"prove,omitempty"`
		Expand    bool        `json:"expand,omitempty"`
		Wait      interface{} `json:"wait,omitempty"`
		Delivered bool        `json:"delivered,omitempty"`
	}{}
	u.Prove = v.Prove
	u.Expand = v.Expand
	u.Wait = encoding.DurationToJSON(v.Wait)
	u.Delivered = v.Delivered
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Prove = u.Prove
	v.Expand = u.Expand
	if x, err := encoding.DurationFromJSON(u.Wait); err != nil {
		return fmt.Errorf("error decoding Wait: %w", err)
	} else {
		v.Wait = x
	}
	v.Delivered = u.Delivered
	return nil
}

func (v *Submission) UnmarshalJSON(data []byte) error {
	u := struct {
		TransactionHashes encoding.JsonList[string]                      `json:"transactionHashes,omitempty"`
		SignatureHashes   encoding.JsonList[string]                      `json:"signatureHashes,omitempty"`
		Status            encoding.JsonList[*protocol.TransactionStatus] `json:"status,omitempty"`
	}{}
	u.TransactionHashes = make(encoding.JsonList[string], len(v.TransactionHashes))
	for i, x := range v.TransactionHashes {
		u.TransactionHashes[i] = encoding.ChainToJSON(x)
	}
	u.SignatureHashes = make(encoding.JsonList[string], len(v.SignatureHashes))
	for i, x := range v.SignatureHashes {
		u.SignatureHashes[i] = encoding.ChainToJSON(x)
	}
	u.Status = v.Status
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.TransactionHashes = make([][32]byte, len(u.TransactionHashes))
	for i, x := range u.TransactionHashes {
		if x, err := encoding.ChainFromJSON(x); err != nil {
			return fmt.Errorf("error decoding TransactionHashes: %w", err)
		} else {
			v.TransactionHashes[i] = x
		}
	}
	v.SignatureHashes = make([][32]byte, len(u.SignatureHashes))
	for i, x := range u.SignatureHashes {
		if x, err := encoding.ChainFromJSON(x); err != nil {
			return fmt.Errorf("error decoding SignatureHashes: %w", err)
		} else {
			v.SignatureHashes[i] = x
		}
	}
	v.Status = u.Status
	return nil
}

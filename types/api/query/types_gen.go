package query

// GENERATED BY go run ./tools/cmd/genmarshal. DO NOT EDIT.

import (
	"bytes"
	"encoding/json"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type MultiResponse struct {
	Type  string   `json:"type,omitempty" form:"type" query:"type" validate:"required"`
	Items []string `json:"items,omitempty" form:"items" query:"items" validate:"required"`
	Start uint64   `json:"start" form:"start" query:"start" validate:"required"`
	Count uint64   `json:"count" form:"count" query:"count" validate:"required"`
	Total uint64   `json:"total" form:"total" query:"total" validate:"required"`
}

type RequestKeyPageIndex struct {
	Url string `json:"url,omitempty" form:"url" query:"url" validate:"required,acc-url"`
	Key []byte `json:"key,omitempty" form:"key" query:"key" validate:"required"`
}

type ResponseByTxId struct {
	TxId           [32]byte     `json:"txId,omitempty" form:"txId" query:"txId" validate:"required"`
	TxState        []byte       `json:"txState,omitempty" form:"txState" query:"txState" validate:"required"`
	TxPendingState []byte       `json:"txPendingState,omitempty" form:"txPendingState" query:"txPendingState" validate:"required"`
	TxSynthTxIds   []byte       `json:"txSynthTxIds,omitempty" form:"txSynthTxIds" query:"txSynthTxIds" validate:"required"`
	Height         int64        `json:"height" form:"height" query:"height" validate:"required"`
	ChainState     [][]byte     `json:"chainState,omitempty" form:"chainState" query:"chainState" validate:"required"`
	Receipts       []*TxReceipt `json:"receipts,omitempty" form:"receipts" query:"receipts" validate:"required"`
}

type ResponseChainEntry struct {
	Height int64    `json:"height" form:"height" query:"height" validate:"required"`
	Entry  []byte   `json:"entry,omitempty" form:"entry" query:"entry" validate:"required"`
	State  [][]byte `json:"state,omitempty" form:"state" query:"state" validate:"required"`
}

type ResponseChainRange struct {
	Start   int64    `json:"start" form:"start" query:"start" validate:"required"`
	End     int64    `json:"end" form:"end" query:"end" validate:"required"`
	Total   int64    `json:"total" form:"total" query:"total" validate:"required"`
	Entries [][]byte `json:"entries,omitempty" form:"entries" query:"entries" validate:"required"`
}

type ResponseKeyPageIndex struct {
	KeyBook string `json:"keyBook,omitempty" form:"keyBook" query:"keyBook" validate:"required"`
	KeyPage string `json:"keyPage,omitempty" form:"keyPage" query:"keyPage" validate:"required"`
	Index   uint64 `json:"index" form:"index" query:"index" validate:"required"`
}

type ResponsePending struct {
	Transactions [][32]byte `json:"transactions,omitempty" form:"transactions" query:"transactions" validate:"required"`
}

type ResponseTxHistory struct {
	Start        int64            `json:"start" form:"start" query:"start" validate:"required"`
	End          int64            `json:"end" form:"end" query:"end" validate:"required"`
	Total        int64            `json:"total" form:"total" query:"total" validate:"required"`
	Transactions []ResponseByTxId `json:"transactions,omitempty" form:"transactions" query:"transactions" validate:"required"`
}

type TxReceipt struct {
	Account        *url.URL         `json:"account,omitempty" form:"account" query:"account" validate:"required"`
	Chain          string           `json:"chain,omitempty" form:"chain" query:"chain" validate:"required"`
	DirectoryBlock uint64           `json:"directoryBlock,omitempty" form:"directoryBlock" query:"directoryBlock" validate:"required"`
	Receipt        protocol.Receipt `json:"receipt,omitempty" form:"receipt" query:"receipt" validate:"required"`
}

func (v *RequestKeyPageIndex) Equal(u *RequestKeyPageIndex) bool {
	if !(v.Url == u.Url) {
		return false
	}

	if !(bytes.Equal(v.Key, u.Key)) {
		return false
	}

	return true
}

func (v *ResponseByTxId) Equal(u *ResponseByTxId) bool {
	if !(v.TxId == u.TxId) {
		return false
	}

	if !(bytes.Equal(v.TxState, u.TxState)) {
		return false
	}

	if !(bytes.Equal(v.TxPendingState, u.TxPendingState)) {
		return false
	}

	if !(bytes.Equal(v.TxSynthTxIds, u.TxSynthTxIds)) {
		return false
	}

	if !(v.Height == u.Height) {
		return false
	}

	if !(len(v.ChainState) == len(u.ChainState)) {
		return false
	}

	for i := range v.ChainState {
		v, u := v.ChainState[i], u.ChainState[i]
		if !(bytes.Equal(v, u)) {
			return false
		}

	}

	if !(len(v.Receipts) == len(u.Receipts)) {
		return false
	}

	for i := range v.Receipts {
		v, u := v.Receipts[i], u.Receipts[i]
		if !(v.Equal(u)) {
			return false
		}

	}

	return true
}

func (v *ResponseChainEntry) Equal(u *ResponseChainEntry) bool {
	if !(v.Height == u.Height) {
		return false
	}

	if !(bytes.Equal(v.Entry, u.Entry)) {
		return false
	}

	if !(len(v.State) == len(u.State)) {
		return false
	}

	for i := range v.State {
		v, u := v.State[i], u.State[i]
		if !(bytes.Equal(v, u)) {
			return false
		}

	}

	return true
}

func (v *ResponseChainRange) Equal(u *ResponseChainRange) bool {
	if !(v.Start == u.Start) {
		return false
	}

	if !(v.End == u.End) {
		return false
	}

	if !(v.Total == u.Total) {
		return false
	}

	if !(len(v.Entries) == len(u.Entries)) {
		return false
	}

	for i := range v.Entries {
		v, u := v.Entries[i], u.Entries[i]
		if !(bytes.Equal(v, u)) {
			return false
		}

	}

	return true
}

func (v *ResponseKeyPageIndex) Equal(u *ResponseKeyPageIndex) bool {
	if !(v.KeyBook == u.KeyBook) {
		return false
	}

	if !(v.KeyPage == u.KeyPage) {
		return false
	}

	if !(v.Index == u.Index) {
		return false
	}

	return true
}

func (v *ResponsePending) Equal(u *ResponsePending) bool {
	if !(len(v.Transactions) == len(u.Transactions)) {
		return false
	}

	for i := range v.Transactions {
		if v.Transactions[i] != u.Transactions[i] {
			return false
		}
	}

	return true
}

func (v *ResponseTxHistory) Equal(u *ResponseTxHistory) bool {
	if !(v.Start == u.Start) {
		return false
	}

	if !(v.End == u.End) {
		return false
	}

	if !(v.Total == u.Total) {
		return false
	}

	if !(len(v.Transactions) == len(u.Transactions)) {
		return false
	}

	for i := range v.Transactions {
		v, u := v.Transactions[i], u.Transactions[i]
		if !(v.Equal(&u)) {
			return false
		}

	}

	return true
}

func (v *TxReceipt) Equal(u *TxReceipt) bool {
	if !(v.Account.Equal(u.Account)) {
		return false
	}

	if !(v.Chain == u.Chain) {
		return false
	}

	if !(v.DirectoryBlock == u.DirectoryBlock) {
		return false
	}

	if !(v.Receipt.Equal(&u.Receipt)) {
		return false
	}

	return true
}

func (v *MultiResponse) BinarySize() int {
	var n int

	n += encoding.StringBinarySize(v.Type)

	n += encoding.UvarintBinarySize(uint64(len(v.Items)))

	for _, v := range v.Items {
		n += encoding.StringBinarySize(v)

	}

	n += encoding.UvarintBinarySize(v.Start)

	n += encoding.UvarintBinarySize(v.Count)

	n += encoding.UvarintBinarySize(v.Total)

	return n
}

func (v *RequestKeyPageIndex) BinarySize() int {
	var n int

	n += encoding.StringBinarySize(v.Url)

	n += encoding.BytesBinarySize(v.Key)

	return n
}

func (v *ResponseByTxId) BinarySize() int {
	var n int

	n += encoding.ChainBinarySize(&v.TxId)

	n += encoding.BytesBinarySize(v.TxState)

	n += encoding.BytesBinarySize(v.TxPendingState)

	n += encoding.BytesBinarySize(v.TxSynthTxIds)

	n += encoding.VarintBinarySize(v.Height)

	n += encoding.UvarintBinarySize(uint64(len(v.ChainState)))

	for _, v := range v.ChainState {
		n += encoding.BytesBinarySize(v)

	}

	n += encoding.UvarintBinarySize(uint64(len(v.Receipts)))

	for _, v := range v.Receipts {
		n += v.BinarySize()

	}

	return n
}

func (v *ResponseChainEntry) BinarySize() int {
	var n int

	n += encoding.VarintBinarySize(v.Height)

	n += encoding.BytesBinarySize(v.Entry)

	n += encoding.UvarintBinarySize(uint64(len(v.State)))

	for _, v := range v.State {
		n += encoding.BytesBinarySize(v)

	}

	return n
}

func (v *ResponseChainRange) BinarySize() int {
	var n int

	n += encoding.VarintBinarySize(v.Start)

	n += encoding.VarintBinarySize(v.End)

	n += encoding.VarintBinarySize(v.Total)

	n += encoding.UvarintBinarySize(uint64(len(v.Entries)))

	for _, v := range v.Entries {
		n += encoding.BytesBinarySize(v)

	}

	return n
}

func (v *ResponseKeyPageIndex) BinarySize() int {
	var n int

	n += encoding.StringBinarySize(v.KeyBook)

	n += encoding.StringBinarySize(v.KeyPage)

	n += encoding.UvarintBinarySize(v.Index)

	return n
}

func (v *ResponsePending) BinarySize() int {
	var n int

	n += encoding.ChainSetBinarySize(v.Transactions)

	return n
}

func (v *ResponseTxHistory) BinarySize() int {
	var n int

	n += encoding.VarintBinarySize(v.Start)

	n += encoding.VarintBinarySize(v.End)

	n += encoding.VarintBinarySize(v.Total)

	n += encoding.UvarintBinarySize(uint64(len(v.Transactions)))

	for _, v := range v.Transactions {
		n += v.BinarySize()

	}

	return n
}

func (v *TxReceipt) BinarySize() int {
	var n int

	n += v.Account.BinarySize()

	n += encoding.StringBinarySize(v.Chain)

	n += encoding.UvarintBinarySize(v.DirectoryBlock)

	n += v.Receipt.BinarySize()

	return n
}

func (v *MultiResponse) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(encoding.StringMarshalBinary(v.Type))

	buffer.Write(encoding.UvarintMarshalBinary(uint64(len(v.Items))))
	for i, v := range v.Items {
		_ = i
		buffer.Write(encoding.StringMarshalBinary(v))

	}

	buffer.Write(encoding.UvarintMarshalBinary(v.Start))

	buffer.Write(encoding.UvarintMarshalBinary(v.Count))

	buffer.Write(encoding.UvarintMarshalBinary(v.Total))

	return buffer.Bytes(), nil
}

func (v *RequestKeyPageIndex) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(encoding.StringMarshalBinary(v.Url))

	buffer.Write(encoding.BytesMarshalBinary(v.Key))

	return buffer.Bytes(), nil
}

func (v *ResponseByTxId) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(encoding.ChainMarshalBinary(&v.TxId))

	buffer.Write(encoding.BytesMarshalBinary(v.TxState))

	buffer.Write(encoding.BytesMarshalBinary(v.TxPendingState))

	buffer.Write(encoding.BytesMarshalBinary(v.TxSynthTxIds))

	buffer.Write(encoding.VarintMarshalBinary(v.Height))

	buffer.Write(encoding.UvarintMarshalBinary(uint64(len(v.ChainState))))
	for i, v := range v.ChainState {
		_ = i
		buffer.Write(encoding.BytesMarshalBinary(v))

	}

	buffer.Write(encoding.UvarintMarshalBinary(uint64(len(v.Receipts))))
	for i, v := range v.Receipts {
		_ = i
		if b, err := v.MarshalBinary(); err != nil {
			return nil, fmt.Errorf("error encoding Receipts[%d]: %w", i, err)
		} else {
			buffer.Write(b)
		}

	}

	return buffer.Bytes(), nil
}

func (v *ResponseChainEntry) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(encoding.VarintMarshalBinary(v.Height))

	buffer.Write(encoding.BytesMarshalBinary(v.Entry))

	buffer.Write(encoding.UvarintMarshalBinary(uint64(len(v.State))))
	for i, v := range v.State {
		_ = i
		buffer.Write(encoding.BytesMarshalBinary(v))

	}

	return buffer.Bytes(), nil
}

func (v *ResponseChainRange) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(encoding.VarintMarshalBinary(v.Start))

	buffer.Write(encoding.VarintMarshalBinary(v.End))

	buffer.Write(encoding.VarintMarshalBinary(v.Total))

	buffer.Write(encoding.UvarintMarshalBinary(uint64(len(v.Entries))))
	for i, v := range v.Entries {
		_ = i
		buffer.Write(encoding.BytesMarshalBinary(v))

	}

	return buffer.Bytes(), nil
}

func (v *ResponseKeyPageIndex) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(encoding.StringMarshalBinary(v.KeyBook))

	buffer.Write(encoding.StringMarshalBinary(v.KeyPage))

	buffer.Write(encoding.UvarintMarshalBinary(v.Index))

	return buffer.Bytes(), nil
}

func (v *ResponsePending) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(encoding.ChainSetMarshalBinary(v.Transactions))

	return buffer.Bytes(), nil
}

func (v *ResponseTxHistory) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	buffer.Write(encoding.VarintMarshalBinary(v.Start))

	buffer.Write(encoding.VarintMarshalBinary(v.End))

	buffer.Write(encoding.VarintMarshalBinary(v.Total))

	buffer.Write(encoding.UvarintMarshalBinary(uint64(len(v.Transactions))))
	for i, v := range v.Transactions {
		_ = i
		if b, err := v.MarshalBinary(); err != nil {
			return nil, fmt.Errorf("error encoding Transactions[%d]: %w", i, err)
		} else {
			buffer.Write(b)
		}

	}

	return buffer.Bytes(), nil
}

func (v *TxReceipt) MarshalBinary() ([]byte, error) {
	var buffer bytes.Buffer

	if b, err := v.Account.MarshalBinary(); err != nil {
		return nil, fmt.Errorf("error encoding Account: %w", err)
	} else {
		buffer.Write(b)
	}

	buffer.Write(encoding.StringMarshalBinary(v.Chain))

	buffer.Write(encoding.UvarintMarshalBinary(v.DirectoryBlock))

	if b, err := v.Receipt.MarshalBinary(); err != nil {
		return nil, fmt.Errorf("error encoding Receipt: %w", err)
	} else {
		buffer.Write(b)
	}

	return buffer.Bytes(), nil
}

func (v *MultiResponse) UnmarshalBinary(data []byte) error {
	if x, err := encoding.StringUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Type: %w", err)
	} else {
		v.Type = x
	}
	data = data[encoding.StringBinarySize(v.Type):]

	var lenItems uint64
	if x, err := encoding.UvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Items: %w", err)
	} else {
		lenItems = x
	}
	data = data[encoding.UvarintBinarySize(lenItems):]

	v.Items = make([]string, lenItems)
	for i := range v.Items {
		if x, err := encoding.StringUnmarshalBinary(data); err != nil {
			return fmt.Errorf("error decoding Items[%d]: %w", i, err)
		} else {
			v.Items[i] = x
		}
		data = data[encoding.StringBinarySize(v.Items[i]):]

	}

	if x, err := encoding.UvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Start: %w", err)
	} else {
		v.Start = x
	}
	data = data[encoding.UvarintBinarySize(v.Start):]

	if x, err := encoding.UvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Count: %w", err)
	} else {
		v.Count = x
	}
	data = data[encoding.UvarintBinarySize(v.Count):]

	if x, err := encoding.UvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Total: %w", err)
	} else {
		v.Total = x
	}
	data = data[encoding.UvarintBinarySize(v.Total):]

	return nil
}

func (v *RequestKeyPageIndex) UnmarshalBinary(data []byte) error {
	if x, err := encoding.StringUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Url: %w", err)
	} else {
		v.Url = x
	}
	data = data[encoding.StringBinarySize(v.Url):]

	if x, err := encoding.BytesUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Key: %w", err)
	} else {
		v.Key = x
	}
	data = data[encoding.BytesBinarySize(v.Key):]

	return nil
}

func (v *ResponseByTxId) UnmarshalBinary(data []byte) error {
	if x, err := encoding.ChainUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding TxId: %w", err)
	} else {
		v.TxId = x
	}
	data = data[encoding.ChainBinarySize(&v.TxId):]

	if x, err := encoding.BytesUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding TxState: %w", err)
	} else {
		v.TxState = x
	}
	data = data[encoding.BytesBinarySize(v.TxState):]

	if x, err := encoding.BytesUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding TxPendingState: %w", err)
	} else {
		v.TxPendingState = x
	}
	data = data[encoding.BytesBinarySize(v.TxPendingState):]

	if x, err := encoding.BytesUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding TxSynthTxIds: %w", err)
	} else {
		v.TxSynthTxIds = x
	}
	data = data[encoding.BytesBinarySize(v.TxSynthTxIds):]

	if x, err := encoding.VarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Height: %w", err)
	} else {
		v.Height = x
	}
	data = data[encoding.VarintBinarySize(v.Height):]

	var lenChainState uint64
	if x, err := encoding.UvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding ChainState: %w", err)
	} else {
		lenChainState = x
	}
	data = data[encoding.UvarintBinarySize(lenChainState):]

	v.ChainState = make([][]byte, lenChainState)
	for i := range v.ChainState {
		if x, err := encoding.BytesUnmarshalBinary(data); err != nil {
			return fmt.Errorf("error decoding ChainState[%d]: %w", i, err)
		} else {
			v.ChainState[i] = x
		}
		data = data[encoding.BytesBinarySize(v.ChainState[i]):]

	}

	var lenReceipts uint64
	if x, err := encoding.UvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Receipts: %w", err)
	} else {
		lenReceipts = x
	}
	data = data[encoding.UvarintBinarySize(lenReceipts):]

	v.Receipts = make([]*TxReceipt, lenReceipts)
	for i := range v.Receipts {
		var x *TxReceipt
		x = new(TxReceipt)
		if err := x.UnmarshalBinary(data); err != nil {
			return fmt.Errorf("error decoding Receipts[%d]: %w", i, err)
		}
		data = data[x.BinarySize():]

		v.Receipts[i] = x
	}

	return nil
}

func (v *ResponseChainEntry) UnmarshalBinary(data []byte) error {
	if x, err := encoding.VarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Height: %w", err)
	} else {
		v.Height = x
	}
	data = data[encoding.VarintBinarySize(v.Height):]

	if x, err := encoding.BytesUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Entry: %w", err)
	} else {
		v.Entry = x
	}
	data = data[encoding.BytesBinarySize(v.Entry):]

	var lenState uint64
	if x, err := encoding.UvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding State: %w", err)
	} else {
		lenState = x
	}
	data = data[encoding.UvarintBinarySize(lenState):]

	v.State = make([][]byte, lenState)
	for i := range v.State {
		if x, err := encoding.BytesUnmarshalBinary(data); err != nil {
			return fmt.Errorf("error decoding State[%d]: %w", i, err)
		} else {
			v.State[i] = x
		}
		data = data[encoding.BytesBinarySize(v.State[i]):]

	}

	return nil
}

func (v *ResponseChainRange) UnmarshalBinary(data []byte) error {
	if x, err := encoding.VarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Start: %w", err)
	} else {
		v.Start = x
	}
	data = data[encoding.VarintBinarySize(v.Start):]

	if x, err := encoding.VarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding End: %w", err)
	} else {
		v.End = x
	}
	data = data[encoding.VarintBinarySize(v.End):]

	if x, err := encoding.VarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Total: %w", err)
	} else {
		v.Total = x
	}
	data = data[encoding.VarintBinarySize(v.Total):]

	var lenEntries uint64
	if x, err := encoding.UvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Entries: %w", err)
	} else {
		lenEntries = x
	}
	data = data[encoding.UvarintBinarySize(lenEntries):]

	v.Entries = make([][]byte, lenEntries)
	for i := range v.Entries {
		if x, err := encoding.BytesUnmarshalBinary(data); err != nil {
			return fmt.Errorf("error decoding Entries[%d]: %w", i, err)
		} else {
			v.Entries[i] = x
		}
		data = data[encoding.BytesBinarySize(v.Entries[i]):]

	}

	return nil
}

func (v *ResponseKeyPageIndex) UnmarshalBinary(data []byte) error {
	if x, err := encoding.StringUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding KeyBook: %w", err)
	} else {
		v.KeyBook = x
	}
	data = data[encoding.StringBinarySize(v.KeyBook):]

	if x, err := encoding.StringUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding KeyPage: %w", err)
	} else {
		v.KeyPage = x
	}
	data = data[encoding.StringBinarySize(v.KeyPage):]

	if x, err := encoding.UvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Index: %w", err)
	} else {
		v.Index = x
	}
	data = data[encoding.UvarintBinarySize(v.Index):]

	return nil
}

func (v *ResponsePending) UnmarshalBinary(data []byte) error {
	if x, err := encoding.ChainSetUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Transactions: %w", err)
	} else {
		v.Transactions = x
	}
	data = data[encoding.ChainSetBinarySize(v.Transactions):]

	return nil
}

func (v *ResponseTxHistory) UnmarshalBinary(data []byte) error {
	if x, err := encoding.VarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Start: %w", err)
	} else {
		v.Start = x
	}
	data = data[encoding.VarintBinarySize(v.Start):]

	if x, err := encoding.VarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding End: %w", err)
	} else {
		v.End = x
	}
	data = data[encoding.VarintBinarySize(v.End):]

	if x, err := encoding.VarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Total: %w", err)
	} else {
		v.Total = x
	}
	data = data[encoding.VarintBinarySize(v.Total):]

	var lenTransactions uint64
	if x, err := encoding.UvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Transactions: %w", err)
	} else {
		lenTransactions = x
	}
	data = data[encoding.UvarintBinarySize(lenTransactions):]

	v.Transactions = make([]ResponseByTxId, lenTransactions)
	for i := range v.Transactions {
		if err := v.Transactions[i].UnmarshalBinary(data); err != nil {
			return fmt.Errorf("error decoding Transactions[%d]: %w", i, err)
		}
		data = data[v.Transactions[i].BinarySize():]

	}

	return nil
}

func (v *TxReceipt) UnmarshalBinary(data []byte) error {
	v.Account = new(url.URL)
	if err := v.Account.UnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Account: %w", err)
	}
	data = data[v.Account.BinarySize():]

	if x, err := encoding.StringUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Chain: %w", err)
	} else {
		v.Chain = x
	}
	data = data[encoding.StringBinarySize(v.Chain):]

	if x, err := encoding.UvarintUnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding DirectoryBlock: %w", err)
	} else {
		v.DirectoryBlock = x
	}
	data = data[encoding.UvarintBinarySize(v.DirectoryBlock):]

	if err := v.Receipt.UnmarshalBinary(data); err != nil {
		return fmt.Errorf("error decoding Receipt: %w", err)
	}
	data = data[v.Receipt.BinarySize():]

	return nil
}

func (v *RequestKeyPageIndex) MarshalJSON() ([]byte, error) {
	u := struct {
		Url string  `json:"url,omitempty"`
		Key *string `json:"key,omitempty"`
	}{}
	u.Url = v.Url
	u.Key = encoding.BytesToJSON(v.Key)
	return json.Marshal(&u)
}

func (v *ResponseByTxId) MarshalJSON() ([]byte, error) {
	u := struct {
		TxId           string       `json:"txId,omitempty"`
		TxState        *string      `json:"txState,omitempty"`
		TxPendingState *string      `json:"txPendingState,omitempty"`
		TxSynthTxIds   *string      `json:"txSynthTxIds,omitempty"`
		Height         int64        `json:"height"`
		ChainState     []*string    `json:"chainState,omitempty"`
		Receipts       []*TxReceipt `json:"receipts,omitempty"`
	}{}
	u.TxId = encoding.ChainToJSON(v.TxId)
	u.TxState = encoding.BytesToJSON(v.TxState)
	u.TxPendingState = encoding.BytesToJSON(v.TxPendingState)
	u.TxSynthTxIds = encoding.BytesToJSON(v.TxSynthTxIds)
	u.Height = v.Height
	u.ChainState = make([]*string, len(v.ChainState))
	for i, x := range v.ChainState {
		u.ChainState[i] = encoding.BytesToJSON(x)
	}
	u.Receipts = v.Receipts
	return json.Marshal(&u)
}

func (v *ResponseChainEntry) MarshalJSON() ([]byte, error) {
	u := struct {
		Height int64     `json:"height"`
		Entry  *string   `json:"entry,omitempty"`
		State  []*string `json:"state,omitempty"`
	}{}
	u.Height = v.Height
	u.Entry = encoding.BytesToJSON(v.Entry)
	u.State = make([]*string, len(v.State))
	for i, x := range v.State {
		u.State[i] = encoding.BytesToJSON(x)
	}
	return json.Marshal(&u)
}

func (v *ResponseChainRange) MarshalJSON() ([]byte, error) {
	u := struct {
		Start   int64     `json:"start"`
		End     int64     `json:"end"`
		Total   int64     `json:"total"`
		Entries []*string `json:"entries,omitempty"`
	}{}
	u.Start = v.Start
	u.End = v.End
	u.Total = v.Total
	u.Entries = make([]*string, len(v.Entries))
	for i, x := range v.Entries {
		u.Entries[i] = encoding.BytesToJSON(x)
	}
	return json.Marshal(&u)
}

func (v *ResponsePending) MarshalJSON() ([]byte, error) {
	u := struct {
		Transactions []string `json:"transactions,omitempty"`
	}{}
	u.Transactions = encoding.ChainSetToJSON(v.Transactions)
	return json.Marshal(&u)
}

func (v *RequestKeyPageIndex) UnmarshalJSON(data []byte) error {
	u := struct {
		Url string  `json:"url,omitempty"`
		Key *string `json:"key,omitempty"`
	}{}
	u.Url = v.Url
	u.Key = encoding.BytesToJSON(v.Key)
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Url = u.Url
	if x, err := encoding.BytesFromJSON(u.Key); err != nil {
		return fmt.Errorf("error decoding Key: %w", err)
	} else {
		v.Key = x
	}
	return nil
}

func (v *ResponseByTxId) UnmarshalJSON(data []byte) error {
	u := struct {
		TxId           string       `json:"txId,omitempty"`
		TxState        *string      `json:"txState,omitempty"`
		TxPendingState *string      `json:"txPendingState,omitempty"`
		TxSynthTxIds   *string      `json:"txSynthTxIds,omitempty"`
		Height         int64        `json:"height"`
		ChainState     []*string    `json:"chainState,omitempty"`
		Receipts       []*TxReceipt `json:"receipts,omitempty"`
	}{}
	u.TxId = encoding.ChainToJSON(v.TxId)
	u.TxState = encoding.BytesToJSON(v.TxState)
	u.TxPendingState = encoding.BytesToJSON(v.TxPendingState)
	u.TxSynthTxIds = encoding.BytesToJSON(v.TxSynthTxIds)
	u.Height = v.Height
	u.ChainState = make([]*string, len(v.ChainState))
	for i, x := range v.ChainState {
		u.ChainState[i] = encoding.BytesToJSON(x)
	}
	u.Receipts = v.Receipts
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	if x, err := encoding.ChainFromJSON(u.TxId); err != nil {
		return fmt.Errorf("error decoding TxId: %w", err)
	} else {
		v.TxId = x
	}
	if x, err := encoding.BytesFromJSON(u.TxState); err != nil {
		return fmt.Errorf("error decoding TxState: %w", err)
	} else {
		v.TxState = x
	}
	if x, err := encoding.BytesFromJSON(u.TxPendingState); err != nil {
		return fmt.Errorf("error decoding TxPendingState: %w", err)
	} else {
		v.TxPendingState = x
	}
	if x, err := encoding.BytesFromJSON(u.TxSynthTxIds); err != nil {
		return fmt.Errorf("error decoding TxSynthTxIds: %w", err)
	} else {
		v.TxSynthTxIds = x
	}
	v.Height = u.Height
	v.ChainState = make([][]byte, len(u.ChainState))
	for i, x := range u.ChainState {
		if x, err := encoding.BytesFromJSON(x); err != nil {
			return fmt.Errorf("error decoding ChainState[%d]: %w", i, err)
		} else {
			v.ChainState[i] = x
		}
	}
	v.Receipts = u.Receipts
	return nil
}

func (v *ResponseChainEntry) UnmarshalJSON(data []byte) error {
	u := struct {
		Height int64     `json:"height"`
		Entry  *string   `json:"entry,omitempty"`
		State  []*string `json:"state,omitempty"`
	}{}
	u.Height = v.Height
	u.Entry = encoding.BytesToJSON(v.Entry)
	u.State = make([]*string, len(v.State))
	for i, x := range v.State {
		u.State[i] = encoding.BytesToJSON(x)
	}
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Height = u.Height
	if x, err := encoding.BytesFromJSON(u.Entry); err != nil {
		return fmt.Errorf("error decoding Entry: %w", err)
	} else {
		v.Entry = x
	}
	v.State = make([][]byte, len(u.State))
	for i, x := range u.State {
		if x, err := encoding.BytesFromJSON(x); err != nil {
			return fmt.Errorf("error decoding State[%d]: %w", i, err)
		} else {
			v.State[i] = x
		}
	}
	return nil
}

func (v *ResponseChainRange) UnmarshalJSON(data []byte) error {
	u := struct {
		Start   int64     `json:"start"`
		End     int64     `json:"end"`
		Total   int64     `json:"total"`
		Entries []*string `json:"entries,omitempty"`
	}{}
	u.Start = v.Start
	u.End = v.End
	u.Total = v.Total
	u.Entries = make([]*string, len(v.Entries))
	for i, x := range v.Entries {
		u.Entries[i] = encoding.BytesToJSON(x)
	}
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	v.Start = u.Start
	v.End = u.End
	v.Total = u.Total
	v.Entries = make([][]byte, len(u.Entries))
	for i, x := range u.Entries {
		if x, err := encoding.BytesFromJSON(x); err != nil {
			return fmt.Errorf("error decoding Entries[%d]: %w", i, err)
		} else {
			v.Entries[i] = x
		}
	}
	return nil
}

func (v *ResponsePending) UnmarshalJSON(data []byte) error {
	u := struct {
		Transactions []string `json:"transactions,omitempty"`
	}{}
	u.Transactions = encoding.ChainSetToJSON(v.Transactions)
	if err := json.Unmarshal(data, &u); err != nil {
		return err
	}
	if x, err := encoding.ChainSetFromJSON(u.Transactions); err != nil {
		return fmt.Errorf("error decoding Transactions: %w", err)
	} else {
		v.Transactions = x
	}
	return nil
}

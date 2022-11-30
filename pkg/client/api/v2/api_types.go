// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package client

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2/query"
)

// Types exported from internal/api/v2
type (
	ChainEntry                  = api.ChainEntry
	ChainIdQuery                = api.ChainIdQuery
	ChainQueryResponse          = api.ChainQueryResponse
	DataEntryQuery              = api.DataEntryQuery
	DataEntryQueryResponse      = api.DataEntryQueryResponse
	DataEntrySetQuery           = api.DataEntrySetQuery
	DescriptionResponse         = api.DescriptionResponse
	DirectoryQuery              = api.DirectoryQuery
	ExecuteRequest              = api.ExecuteRequest
	GeneralQuery                = api.GeneralQuery
	KeyPage                     = api.KeyPage
	KeyPageIndexQuery           = api.KeyPageIndexQuery
	MajorBlocksQuery            = api.MajorBlocksQuery
	MajorQueryResponse          = api.MajorQueryResponse
	MerkleState                 = api.MerkleState
	MetricsQuery                = api.MetricsQuery
	MetricsResponse             = api.MetricsResponse
	MinorBlock                  = api.MinorBlock
	MinorBlocksQuery            = api.MinorBlocksQuery
	MinorQueryResponse          = api.MinorQueryResponse
	MultiResponse               = api.MultiResponse
	QueryOptions                = api.QueryOptions
	QueryPagination             = api.QueryPagination
	SignatureBook               = api.SignatureBook
	SignaturePage               = api.SignaturePage
	Signer                      = api.Signer
	SignerMetadata              = api.SignerMetadata
	StatusResponse              = api.StatusResponse
	SyntheticTransactionRequest = api.SyntheticTransactionRequest
	TokenDeposit                = api.TokenDeposit
	TokenSend                   = api.TokenSend
	TransactionQueryResponse    = api.TransactionQueryResponse
	TxHistoryQuery              = api.TxHistoryQuery
	TxnQuery                    = api.TxnQuery
	TxRequest                   = api.TxRequest
	TxResponse                  = api.TxResponse
	UrlQuery                    = api.UrlQuery
	VersionResponse             = api.VersionResponse
)

// Types exported from internal/api/v2/query
type (
	BlockFilterMode      = query.BlockFilterMode
	ChainState           = query.ChainState
	GeneralReceipt       = query.GeneralReceipt
	ResponseDataEntry    = query.ResponseDataEntry
	ResponseDataEntrySet = query.ResponseDataEntrySet
	ResponseKeyPageIndex = query.ResponseKeyPageIndex
	TxFetchMode          = query.TxFetchMode
	TxReceipt            = query.TxReceipt
)

// Enums exported from internal/api/v2/query
const (
	BlockFilterModeExcludeNone  = query.BlockFilterModeExcludeNone
	BlockFilterModeExcludeEmpty = query.BlockFilterModeExcludeEmpty
	TxFetchModeExpand           = query.TxFetchModeExpand
	TxFetchModeIds              = query.TxFetchModeIds
	TxFetchModeCountOnly        = query.TxFetchModeCountOnly
	TxFetchModeOmit             = query.TxFetchModeOmit
)

func BlockFilterModeByName(name string) (BlockFilterMode, bool) {
	return query.BlockFilterModeByName(name)
}

func TxFetchModeByName(name string) (TxFetchMode, bool) {
	return query.TxFetchModeByName(name)
}

// Errors exported from internal/api/v2
var (
	ErrInternal           = api.ErrInternal
	ErrCanceled           = api.ErrCanceled
	ErrMetricsNotAVector  = api.ErrMetricsNotAVector
	ErrMetricsVectorEmpty = api.ErrMetricsVectorEmpty
	ErrInvalidUrl         = api.ErrInvalidUrl
)

// Error codes exported from internal/api/v2
const (
	ErrCodeInternal           = api.ErrCodeInternal
	ErrCodeDispatch           = api.ErrCodeDispatch
	ErrCodeValidation         = api.ErrCodeValidation
	ErrCodeSubmission         = api.ErrCodeSubmission
	ErrCodeAccumulate         = api.ErrCodeAccumulate
	ErrCodeNotLiteAccount     = api.ErrCodeNotLiteAccount
	ErrCodeNotAcmeAccount     = api.ErrCodeNotAcmeAccount
	ErrCodeNotFound           = api.ErrCodeNotFound
	ErrCodeCanceled           = api.ErrCodeCanceled
	ErrCodeMetricsQuery       = api.ErrCodeMetricsQuery
	ErrCodeMetricsNotAVector  = api.ErrCodeMetricsNotAVector
	ErrCodeMetricsVectorEmpty = api.ErrCodeMetricsVectorEmpty
	ErrCodeProtocolBase       = api.ErrCodeProtocolBase
)

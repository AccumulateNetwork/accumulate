// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package client

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
)

// Types exported from internal/api/v2
type (
	ChainEntry                  = api.ChainEntry
	ChainIdQuery                = api.ChainIdQuery
	ChainQueryResponse          = api.ChainQueryResponse
	DataEntryQuery              = api.DataEntryQuery
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
	BlockFilterMode      = api.BlockFilterMode
	ChainState           = api.ChainState
	GeneralReceipt       = api.GeneralReceipt
	ResponseDataEntry    = api.ResponseDataEntry
	ResponseDataEntrySet = api.ResponseDataEntrySet
	ResponseKeyPageIndex = api.ResponseKeyPageIndex
	TxFetchMode          = api.TxFetchMode
	TxReceipt            = api.TxReceipt
)

// Enums exported from internal/api/v2/query
const (
	BlockFilterModeExcludeNone  = api.BlockFilterModeExcludeNone
	BlockFilterModeExcludeEmpty = api.BlockFilterModeExcludeEmpty
	TxFetchModeExpand           = api.TxFetchModeExpand
	TxFetchModeIds              = api.TxFetchModeIds
	TxFetchModeCountOnly        = api.TxFetchModeCountOnly
	TxFetchModeOmit             = api.TxFetchModeOmit
)

func BlockFilterModeByName(name string) (BlockFilterMode, bool) {
	return api.BlockFilterModeByName(name)
}

func TxFetchModeByName(name string) (TxFetchMode, bool) {
	return api.TxFetchModeByName(name)
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

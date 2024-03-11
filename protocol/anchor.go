// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type AnchorBody interface {
	TransactionBody
	GetPartitionAnchor() *PartitionAnchor
}

func (a *DirectoryAnchor) GetPartitionAnchor() *PartitionAnchor      { return &a.PartitionAnchor }
func (a *BlockValidatorAnchor) GetPartitionAnchor() *PartitionAnchor { return &a.PartitionAnchor }

func EqualAnchorBody(a, b AnchorBody) bool {
	return EqualTransactionBody(a, b)
}

func unmarshalAnchorBody(body TransactionBody, err error) (AnchorBody, error) {
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if body == nil {
		return nil, nil
	}

	anchor, ok := body.(AnchorBody)
	if !ok {
		return nil, errors.EncodingError.WithFormat("%T is not an anchor body", body)
	}

	return anchor, nil
}

func CopyAnchorBody(v AnchorBody) AnchorBody {
	return v.CopyAsInterface().(AnchorBody)
}

func UnmarshalAnchorBody(b []byte) (AnchorBody, error) {
	body, err := unmarshalAnchorBody(UnmarshalTransactionBody(b))
	return body, errors.UnknownError.Wrap(err)
}

func UnmarshalAnchorBodyFrom(r io.Reader) (AnchorBody, error) {
	body, err := unmarshalAnchorBody(UnmarshalTransactionBodyFrom(r))
	return body, errors.UnknownError.Wrap(err)
}

func UnmarshalAnchorBodyJSON(b []byte) (AnchorBody, error) {
	body, err := unmarshalAnchorBody(UnmarshalTransactionBodyJSON(b))
	return body, errors.UnknownError.Wrap(err)
}

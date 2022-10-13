// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import "gitlab.com/accumulatenetwork/accumulate/internal/errors"

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
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	if body == nil {
		return nil, nil
	}

	anchor, ok := body.(AnchorBody)
	if !ok {
		return nil, errors.Format(errors.StatusEncodingError, "%T is not an anchor body", body)
	}

	return anchor, nil
}

func CopyAnchorBody(v AnchorBody) AnchorBody {
	return v.CopyAsInterface().(AnchorBody)
}

func UnmarshalAnchorBody(b []byte) (AnchorBody, error) {
	body, err := unmarshalAnchorBody(UnmarshalTransactionBody(b))
	return body, errors.Wrap(errors.StatusUnknownError, err)
}

func UnmarshalAnchorBodyJSON(b []byte) (AnchorBody, error) {
	body, err := unmarshalAnchorBody(UnmarshalTransactionBodyJSON(b))
	return body, errors.Wrap(errors.StatusUnknownError, err)
}

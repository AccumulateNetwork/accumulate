package protocol

import "gitlab.com/accumulatenetwork/accumulate/pkg/errors"

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

	anchor, ok := body.(AnchorBody)
	if !ok {
		return nil, errors.Format(errors.StatusEncodingError, "%T is not an anchor body", body)
	}

	return anchor, nil
}

func UnmarshalAnchorBody(b []byte) (AnchorBody, error) {
	body, err := unmarshalAnchorBody(UnmarshalTransactionBody(b))
	return body, errors.Wrap(errors.StatusUnknownError, err)
}

func UnmarshalAnchorBodyJSON(b []byte) (AnchorBody, error) {
	body, err := unmarshalAnchorBody(UnmarshalTransactionBodyJSON(b))
	return body, errors.Wrap(errors.StatusUnknownError, err)
}

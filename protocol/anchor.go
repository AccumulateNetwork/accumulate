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
		return nil, errors.Unknown.Wrap(err)
	}

	anchor, ok := body.(AnchorBody)
	if !ok {
		return nil, errors.Encoding.Format("%T is not an anchor body", body)
	}

	return anchor, nil
}

func UnmarshalAnchorBody(b []byte) (AnchorBody, error) {
	body, err := unmarshalAnchorBody(UnmarshalTransactionBody(b))
	return body, errors.Unknown.Wrap(err)
}

func UnmarshalAnchorBodyJSON(b []byte) (AnchorBody, error) {
	body, err := unmarshalAnchorBody(UnmarshalTransactionBodyJSON(b))
	return body, errors.Unknown.Wrap(err)
}

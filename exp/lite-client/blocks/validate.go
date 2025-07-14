package blocks

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ValidateMajorBlock validates a major block.
func ValidateMajorBlock(ctx context.Context, block *api.MajorBlockRecord, validator *BlockValidator) error {
	// Construct the envelope from the major block and pass it to the validator.
	var anchor *messaging.BlockAnchor
	var signatures []protocol.Signature

	for _, minor := range block.MinorBlocks.Records {
		for _, chainEntry := range minor.Entries.Records {
			switch entry := chainEntry.Value.(type) {
			case *api.MessageRecord[messaging.Message]:
				switch msg := entry.Message.(type) {
				case *messaging.BlockAnchor:
					if anchor == nil {
						anchor = msg
					}
				case messaging.MessageWithSignature:
					signatures = append(signatures, msg.GetSignature())
				}

			case *api.SignatureSetRecord:
				for _, sig := range entry.Signatures.Records {
					msg, ok := sig.Message.(messaging.MessageWithSignature)
					if !ok {
						continue
					}
					signatures = append(signatures, msg.GetSignature())
				}
			}
		}
	}

	if anchor == nil {
		return fmt.Errorf("major block %d is missing its anchor", block.Index)
	}

	hash := anchor.Hash()
	envelope := &messaging.Envelope{
		Signatures: signatures,
		TxHash:     hash[:],
		Messages:   []messaging.Message{anchor},
	}

	_, err := validator.Validate(ctx, envelope, api.ValidateOptions{})
	return err
}

// ValidateMessageRecord uses the v3 Validator interface to validate a MessageRecord.
// It wraps the record in an Envelope and calls Validate.
func ValidateMessageRecord(ctx context.Context, validator api.Validator, rec *api.MessageRecord[messaging.Message]) (bool, error) {
	if rec == nil || rec.Message == nil {
		return false, nil
	}
	env := new(messaging.Envelope)
	env.Messages = []messaging.Message{rec.Message}
	_, err := validator.Validate(ctx, env, api.ValidateOptions{})
	if err != nil {
		return false, err
	}
	// If no error, validation succeeded
	return true, nil
}

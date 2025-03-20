// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package healing

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SequencedInfo struct {
	Source      string
	Destination string
	Number      uint64
	ID          *url.TxID
}

// ResolveSequenced resolves an anchor or synthetic message (a sequenced
// message). If the client's address is non-nil, the query will be sent to that
// address. Otherwise, all of the source partition's nodes will be queried in
// order until one responds.
func ResolveSequenced[T messaging.Message](ctx context.Context, client message.AddressedClient, net *NetworkInfo, srcId, dstId string, seqNum uint64, anchor bool) (*api.MessageRecord[T], error) {
	srcUrl := protocol.PartitionUrl(srcId)
	dstUrl := protocol.PartitionUrl(dstId)

	var account string
	if anchor {
		account = protocol.AnchorPool
	} else {
		account = protocol.Synthetic
	}

	// If the client has an address, use that
	if client.Address != nil {
		slog.InfoContext(ctx, "Querying node", "address", client.Address)
		res, err := client.Private().Sequence(ctx, srcUrl.JoinPath(account), dstUrl, seqNum, private.SequenceOptions{})
		if err != nil {
			return nil, err
		}

		r2, err := api.MessageRecordAs[T](res)
		if err != nil {
			return nil, err
		}
		return r2, nil
	}

	// Otherwise try each node until one succeeds
	slog.InfoContext(ctx, "Resolving the message ID", "source", srcId, "destination", dstId, "number", seqNum)
	if anchor {
		// Limit the number of anchors to process
		anchorLimit := 5
		anchorCount := 0

		// Check if we have peers for this source partition
		peers, ok := net.Peers[strings.ToLower(srcId)]
		if !ok || len(peers) == 0 {
			slog.WarnContext(ctx, "No peers found for source partition", "source", srcId)
			// Continue processing without breaking out or terminating
			return nil, errors.UnknownError.WithFormat("no peers found for %s", srcId)
		}

		for peer := range peers {
			if anchorCount >= anchorLimit {
				slog.InfoContext(ctx, "Anchor limit reached, stopping further processing")
				break
			}
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			slog.InfoContext(ctx, "Querying node", "id", peer)
			res, err := client.ForPeer(peer).Private().Sequence(ctx, srcUrl.JoinPath(account), dstUrl, seqNum, private.SequenceOptions{})
			if err != nil {
				slog.ErrorContext(ctx, "Query failed", "error", err)
				continue
			}

			r2, err := api.MessageRecordAs[T](res)
			if err != nil {
				slog.ErrorContext(ctx, "Query failed", "error", err)
				continue
			}

			anchorCount++ // Increment the anchor count
			return r2, nil
		}
	} else {
		// Limit the number of synthetic transactions to process
		synthLimit := 5
		synthCount := 0

		// Check if we have peers for this source partition
		peers, ok := net.Peers[strings.ToLower(srcId)]
		if !ok || len(peers) == 0 {
			slog.WarnContext(ctx, "No peers found for source partition", "source", srcId)
			// Continue processing without breaking out or terminating
			return nil, errors.UnknownError.WithFormat("no peers found for %s", srcId)
		}

		for peer := range peers {
			if synthCount >= synthLimit {
				slog.InfoContext(ctx, "Synthetic transaction limit reached, stopping further processing")
				break
			}
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			slog.InfoContext(ctx, "Querying node", "id", peer)
			res, err := client.ForPeer(peer).Private().Sequence(ctx, srcUrl.JoinPath(account), dstUrl, seqNum, private.SequenceOptions{})
			if err != nil {
				slog.ErrorContext(ctx, "Query failed", "error", err)
				continue
			}

			r2, err := api.MessageRecordAs[T](res)
			if err != nil {
				slog.ErrorContext(ctx, "Query failed", "error", err)
				continue
			}

			synthCount++ // Increment the synthetic transaction count
			return r2, nil
		}
	}

	return nil, errors.UnknownError.WithFormat("cannot resolve %s→%s #%d", srcId, dstId, seqNum)
}

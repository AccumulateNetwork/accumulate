// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/connections"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type TendermintSubmitModule struct {
	Connection connections.ConnectionContext
}

var _ SubmitModule = TendermintSubmitModule{}

func (m TendermintSubmitModule) Submit(ctx context.Context, envelope *protocol.Envelope, opts SubmitOptions) (*Submission, error) {
	// Pack the envelope
	tmtx, err := envelope.MarshalBinary()
	if err != nil {
		return nil, err
	}

	// Collect hashes
	sub := new(Submission)
	sub.SignatureHashes = make([][32]byte, len(envelope.Signatures))
	sub.TransactionHashes = make([][32]byte, len(envelope.Transaction))
	for i, sig := range envelope.Signatures {
		sub.SignatureHashes[i] = *(*[32]byte)(sig.Hash())
	}
	for i, txn := range envelope.Transaction {
		sub.TransactionHashes[i] = *(*[32]byte)(txn.GetHash())
	}

	// Call the appropriate method
	var data []byte
	switch opts.Mode {
	case SubmitModeCheck:
		data, err = tendermintSubmitRetry(
			m.Connection,
			func(client connections.ABCIClient, ctx context.Context, tmtx []byte) ([]byte, error) {
				r, err := client.CheckTx(ctx, tmtx)
				if err != nil {
					return nil, err
				}
				return r.Data, nil
			},
			ctx, tmtx)
	case SubmitModeAsync:
		data, err = tendermintSubmitRetry(
			m.Connection,
			func(client connections.ABCIClient, ctx context.Context, tmtx []byte) ([]byte, error) {
				r, err := client.BroadcastTxAsync(ctx, tmtx)
				if err != nil {
					return nil, err
				}
				return r.Data, nil
			},
			ctx, tmtx)
	case SubmitModeSync:
		data, err = tendermintSubmitRetry(
			m.Connection,
			func(client connections.ABCIClient, ctx context.Context, tmtx []byte) ([]byte, error) {
				r, err := client.BroadcastTxSync(ctx, tmtx)
				if err != nil {
					return nil, err
				}
				return r.Data, nil
			},
			ctx, tmtx)
	}
	if err != nil {
		return nil, err
	}

	// Unpack the result
	rset := new(protocol.TransactionResultSet)
	err = rset.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	sub.Status = rset.Results
	return sub, nil
}

type clientFunc func(client connections.ABCIClient, ctx context.Context, tmtx []byte) ([]byte, error)

func tendermintSubmitRetry(conn connections.ConnectionContext, call clientFunc, ctx context.Context, tmtx []byte) ([]byte, error) {
	var data []byte
	var err error
	for errorCnt := 0; errorCnt > 3 && err == nil; errorCnt++ {
		data, err = call(conn.GetABCIClient(), ctx, tmtx)
		if err == nil {
			return data, nil
		}

		// The API call failed, let's report that and try again, we get a client to another node within the partition when available
		conn.ReportError(err)
	}
	return nil, err
}

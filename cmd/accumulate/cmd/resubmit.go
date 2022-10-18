// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package cmd

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	resubmitCmd.AddCommand(resubmitAnchorCmd)
}

var resubmitCmd = &cobra.Command{
	Use: "resubmit",
}

var resubmitAnchorCmd = &cobra.Command{
	Use:   "anchor [from-server] [from-url] [to-url] [sequence-number]",
	Short: "Resubmit an anchor",
	Args:  cobra.ExactArgs(4),
	Run:   runCmdFunc(resubmitAnchor),
}

func resubmitAnchor(args []string) (string, error) {
	from, err := client.New(args[0])
	if err != nil {
		return "", err
	}

	fromUrl, err := url.Parse(args[1])
	if err != nil {
		return "", err
	}

	toUrl, err := url.Parse(args[2])
	if err != nil {
		return "", err
	}

	num, err := strconv.ParseUint(args[3], 10, 64)
	if err != nil {
		return "", err
	}

	req1 := new(client.SyntheticTransactionRequest)
	req1.Source = fromUrl
	req1.Destination = toUrl
	req1.SequenceNumber = num
	req1.Anchor = true
	res1, err := from.QuerySynth(context.Background(), req1)
	if err != nil {
		return "", err
	}

	for i, signature := range res1.Signatures {
		switch signature.(type) {
		case *protocol.PartitionSignature:
			res1.Signatures[0], res1.Signatures[i] = res1.Signatures[i], res1.Signatures[0]
		}
	}

	req2 := new(client.ExecuteRequest)
	req2.Envelope = new(protocol.Envelope)
	req2.Envelope.Transaction = []*protocol.Transaction{res1.Transaction}
	req2.Envelope.Signatures = res1.Signatures

	res2, err := Client.ExecuteLocal(context.Background(), req2)
	if err != nil {
		return PrintJsonRpcError(err)
	}
	if res2.Code != 0 {
		result := new(protocol.TransactionStatus)
		if Remarshal(res2.Result, result) != nil {
			return "", errors.New(errors.EncodingError, res2.Message)
		}
		return "", result.Error
	}

	var resps []*client.TransactionQueryResponse
	if TxWait != 0 {
		resps, err = waitForTxnUsingHash(res2.TransactionHash, TxWait, TxIgnorePending)
		if err != nil {
			return PrintJsonRpcError(err)
		}
	}

	result, err := ActionResponseFrom(res2).Print()
	if err != nil {
		return "", err
	}
	if res2.Code == 0 {
		for _, response := range resps {
			str, err := PrintTransactionQueryResponseV2(response)
			if err != nil {
				return PrintJsonRpcError(err)
			}
			result = fmt.Sprint(result, str, "\n")
		}
	}
	return result, nil
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var cmd = &cobra.Command{
	Use:  "resend-anchor [source] [destination] [sequence number]",
	Args: cobra.ExactArgs(3),
	Run:  run,
}

func main() {
	_ = cmd.Execute()
}

var flag = struct {
	Debug bool
}{}

func init() {
	cmd.Flags().BoolVarP(&flag.Debug, "debug", "d", false, "Debug RPC")
}

func run(_ *cobra.Command, args []string) {
	source, err := client.New(args[0])
	checkf(err, "source")

	destination, err := client.New(args[1])
	checkf(err, "destination")

	seqNum, err := strconv.ParseUint(args[2], 10, 64)
	checkf(err, "sequence number")

	if flag.Debug {
		source.DebugRequest = true
		destination.DebugRequest = true
	}

	srcDesc, err := source.Describe(context.Background())
	checkf(err, "query source description")

	dstDesc, err := destination.Describe(context.Background())
	checkf(err, "query destination description")

	src := protocol.PartitionUrl(srcDesc.PartitionId)
	dst := protocol.PartitionUrl(dstDesc.PartitionId)
	querySynthAndExecute(source, destination, src, dst, seqNum)
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func checkf(err error, format string, otherArgs ...interface{}) {
	if err != nil {
		fatalf(format+": %v", append(otherArgs, err)...)
	}
}

func querySynth(c *client.Client, txid *url.TxID, src, dst *url.URL, seqNum uint64) (*protocol.Transaction, []protocol.Signature) {
	req := new(api.SyntheticTransactionRequest)
	req.Source = src
	req.Destination = dst
	req.SequenceNumber = seqNum
	req.Anchor = true
	res, err := c.QuerySynth(context.Background(), req)
	checkf(err, "query anchor %d from %v for %v", seqNum, src, dst)

	if txid != nil && !txid.Equal(res.Transaction.ID()) {
		fatalf("anchor %d from %v for %v has the wrong transaction ID", res.Status.SequenceNumber, src, dst)
	}
	return res.Transaction, res.Signatures
}

func executeLocal(c *client.Client, dst *url.URL, txn *protocol.Transaction, sigs []protocol.Signature) {
	req := new(api.ExecuteRequest)
	req.Envelope = new(protocol.Envelope)
	req.Envelope.Transaction = []*protocol.Transaction{txn}
	req.Envelope.Signatures = sigs
	res2, err := c.ExecuteLocal(context.Background(), req)
	checkf(err, "execute anchor %x from directory for %v", txn.GetHash()[:4], dst)

	if res2.Message != "" {
		fmt.Fprintf(os.Stdout, "Warning: %s\n", res2.Message)
		return
	}

	b, err := json.Marshal(res2.Result)
	checkf(err, "marshal result")
	fmt.Printf("%s\n\n", b)

	// result := new(protocol.TransactionStatus)
	// err = json.Unmarshal(b, result)
	// checkf(err, "unmarshal result")
}

func querySynthAndExecute(csrc, cdst *client.Client, src, dst *url.URL, seqNum uint64) {
	txn, sigs := querySynth(csrc, nil, src, dst, seqNum)

	for i, sig := range sigs {
		if _, ok := sig.(*protocol.PartitionSignature); ok {
			sigs[0], sigs[i] = sigs[i], sigs[0]
			break
		}
	}

	executeLocal(cdst, dst, txn, sigs)
}

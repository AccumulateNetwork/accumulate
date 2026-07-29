// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Command mempoolstat decodes CometBFT mempool contents as Accumulate
// envelopes and reports the message-type mix, so a jam can be attributed to a
// source (anchor re-pushes vs synthetics vs user load) instead of guessed at.
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	url := flag.String("rpc", "http://localhost:26657", "CometBFT RPC endpoint")
	limit := flag.Int("limit", 200, "how many txs to sample")
	file := flag.String("file", "", "read an unconfirmed_txs JSON response from a file instead of the RPC")
	flag.Parse()

	var body struct {
		Result struct {
			NTxs  string   `json:"n_txs"`
			Total string   `json:"total"`
			Txs   []string `json:"txs"`
		} `json:"result"`
	}

	var r io.ReadCloser
	if *file != "" {
		f, err := os.Open(*file)
		if err != nil {
			panic(err)
		}
		r = f
	} else {
		resp, err := http.Get(fmt.Sprintf("%s/unconfirmed_txs?limit=%d", *url, *limit))
		if err != nil {
			panic(err)
		}
		r = resp.Body
	}
	defer r.Close()

	if err := json.NewDecoder(r).Decode(&body); err != nil {
		panic(err)
	}

	msgKind := map[string]int{}
	txnKind := map[string]int{}
	decoded, failed := 0, 0

	for _, b64 := range body.Result.Txs {
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			failed++
			continue
		}
		env := new(messaging.Envelope)
		if err := env.UnmarshalBinary(raw); err != nil {
			failed++
			continue
		}
		decoded++
		msgs, err := env.Normalize()
		if err != nil {
			msgs = env.Messages
		}
		for _, m := range msgs {
			msgKind[m.Type().String()]++
			// Name the underlying transaction where there is one — this is what
			// distinguishes an anchor re-push from ordinary user load.
			switch x := m.(type) {
			case *messaging.TransactionMessage:
				txnKind[x.Transaction.Body.Type().String()]++
			case *messaging.BlockAnchor:
				txnKind["<blockAnchor>"]++
			case *messaging.SequencedMessage:
				if t, ok := x.Message.(*messaging.TransactionMessage); ok {
					txnKind[t.Transaction.Body.Type().String()]++
				}
			case *messaging.SyntheticMessage:
				if s, ok := x.Message.(*messaging.SequencedMessage); ok {
					if t, ok := s.Message.(*messaging.TransactionMessage); ok {
						txnKind[t.Transaction.Body.Type().String()]++
					}
				}
			}
		}
	}

	fmt.Printf("mempool n_txs=%s total=%s   sampled=%d decoded=%d undecodable=%d\n",
		body.Result.NTxs, body.Result.Total, len(body.Result.Txs), decoded, failed)
	dump("message types", msgKind)
	dump("transaction types", txnKind)
	_ = protocol.TransactionTypeUnknown
}

func dump(label string, m map[string]int) {
	type kv struct {
		k string
		v int
	}
	var s []kv
	for k, v := range m {
		s = append(s, kv{k, v})
	}
	sort.Slice(s, func(i, j int) bool { return s[i].v > s[j].v })
	fmt.Printf("\n== %s ==\n", label)
	for _, e := range s {
		fmt.Printf("  %-40s %d\n", e.k, e.v)
	}
}

// blockstore-walk walks every block in a CometBFT blockstore.db and
// emits one JSON record per transaction (or message inside a v2
// envelope), capturing block height, tx hash, principal URL, body
// type, and any obvious "secondary" account touched (e.g. SendTokens
// recipient, SyntheticDeposit recipient, AddCredits recipient).
//
// The output is a JSONL stream — one tx per line — that downstream
// tools can grep / index without having to re-walk the 19M-block
// blockstore.
//
// Usage:
//   blockstore-walk --data-dir <bvnn/data> [--from N] [--to N] [--out file.jsonl]
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	cometbftdb "github.com/cometbft/cometbft-db"
	"github.com/cometbft/cometbft/store"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type record struct {
	Height    int64    `json:"h"`
	Hash      string   `json:"hash"`
	Body      string   `json:"body"`
	Principal string   `json:"principal,omitempty"`
	Touches   []string `json:"touches,omitempty"`
	Signers   []string `json:"signers,omitempty"`
	Source    string   `json:"src,omitempty"` // synth/anchor source partition URL
	Sequence  uint64   `json:"seq,omitempty"`
	Time      string   `json:"t,omitempty"`
}

func main() {
	var (
		dataDir = flag.String("data-dir", "", "directory containing blockstore.db (e.g. .../bvnn/data)")
		from    = flag.Int64("from", 0, "start height (default = base)")
		to      = flag.Int64("to", 0, "end height exclusive (default = height+1)")
		out     = flag.String("out", "", "write JSONL to this file (default stdout)")
		every   = flag.Int64("progress", 100000, "print progress every N blocks")
	)
	flag.Parse()
	if *dataDir == "" {
		fmt.Fprintln(os.Stderr, "--data-dir required")
		os.Exit(1)
	}
	db, err := cometbftdb.NewDB("blockstore", cometbftdb.GoLevelDBBackend, *dataDir)
	must(err)
	defer db.Close()
	bs := store.NewBlockStore(db)
	if *from == 0 {
		*from = bs.Base()
	}
	if *to == 0 {
		*to = bs.Height() + 1
	}

	var w *bufio.Writer
	if *out != "" {
		f, err := os.Create(*out)
		must(err)
		defer f.Close()
		w = bufio.NewWriterSize(f, 256*1024)
		defer w.Flush()
	} else {
		w = bufio.NewWriter(os.Stdout)
		defer w.Flush()
	}
	enc := json.NewEncoder(w)

	fmt.Fprintf(os.Stderr, "walk h=[%d,%d) (height=%d)\n", *from, *to, bs.Height())
	start := time.Now()
	var (
		blocksScanned   int64
		nonemptyBlocks  int64
		txsEmitted      int64
		decodeErrors    int64
		gcEvery         int64 = 200000
	)
	for h := *from; h < *to; h++ {
		blocksScanned++
		if blocksScanned%(*every) == 0 {
			elapsed := time.Since(start).Seconds()
			rate := float64(blocksScanned) / elapsed
			fmt.Fprintf(os.Stderr, "  h=%d (%.0f blk/s, %d non-empty, %d txs, %d decode-err)\n",
				h, rate, atomic.LoadInt64(&nonemptyBlocks), atomic.LoadInt64(&txsEmitted), atomic.LoadInt64(&decodeErrors))
		}
		if blocksScanned%gcEvery == 0 {
			runtime.GC()
		}

		blk := bs.LoadBlock(h)
		if blk == nil || len(blk.Data.Txs) == 0 {
			continue
		}
		nonemptyBlocks++

		for _, raw := range blk.Data.Txs {
			env := new(messaging.Envelope)
			err := env.UnmarshalBinary(raw)
			if err != nil {
				decodeErrors++
				continue
			}
			emitted := emitFromEnvelope(env, h, blk.Time, enc)
			txsEmitted += emitted
			if emitted == 0 {
				// envelope was decodable but contained nothing we recognized
				decodeErrors++
			}
		}
	}
	elapsed := time.Since(start).Seconds()
	fmt.Fprintf(os.Stderr, "done: %d blocks in %.1fs (%.0f blk/s), %d non-empty, %d txs, %d decode-err\n",
		blocksScanned, elapsed, float64(blocksScanned)/elapsed, nonemptyBlocks, txsEmitted, decodeErrors)
}

func emitFromEnvelope(env *messaging.Envelope, h int64, t time.Time, enc *json.Encoder) int64 {
	var emitted int64
	// Collect signer URLs from envelope.Signatures — these accounts
	// are also "touched" by the transaction (LastUsedOn updates,
	// signature chain entries, credit deductions).
	signers := signerURLs(env.Signatures)

	// V1 path: env.Transaction populated
	for _, txn := range env.Transaction {
		emit3(enc, h, t, txn, "", 0, signers)
		emitted++
	}
	// V2 path: env.Messages populated. Each Message can carry a Transaction.
	for _, m := range env.Messages {
		switch v := m.(type) {
		case *messaging.TransactionMessage:
			if v.Transaction != nil {
				emit3(enc, h, t, v.Transaction, "", 0, signers)
				emitted++
			}
		case *messaging.SignatureMessage:
			// V2 signature. The signer is captured separately so a
			// later analyst can see which page/identity signed even
			// when the transaction is processed in a different block.
			if v.Signature == nil {
				continue
			}
			s := v.Signature.GetSigner()
			if s == nil {
				continue
			}
			r := &record{
				Height: h, Body: "signature", Time: t.UTC().Format(time.RFC3339),
				Signers: []string{s.String()},
			}
			_ = enc.Encode(r)
			emitted++
		case *messaging.SequencedMessage:
			src := ""
			if v.Source != nil {
				src = v.Source.String()
			}
			seq := v.Number
			if v.Message == nil {
				continue
			}
			if tm, ok := v.Message.(*messaging.TransactionMessage); ok && tm.Transaction != nil {
				emit3(enc, h, t, tm.Transaction, src, seq, signers)
				emitted++
			}
		case *messaging.SyntheticMessage:
			// Synthetic transactions (deposits, etc.) wrap a
			// SequencedMessage carrying the actual transaction.
			if sm, ok := v.Message.(*messaging.SequencedMessage); ok {
				src := ""
				if sm.Source != nil {
					src = sm.Source.String()
				}
				if tm, ok := sm.Message.(*messaging.TransactionMessage); ok && tm.Transaction != nil {
					emit3(enc, h, t, tm.Transaction, src, sm.Number, signers)
					emitted++
				}
			} else if tm, ok := v.Message.(*messaging.TransactionMessage); ok && tm.Transaction != nil {
				emit3(enc, h, t, tm.Transaction, "", 0, signers)
				emitted++
			}
		case *messaging.BlockAnchor:
			if v.Anchor == nil {
				continue
			}
			if sm, ok := v.Anchor.(*messaging.SequencedMessage); ok {
				src := ""
				if sm.Source != nil {
					src = sm.Source.String()
				}
				r := &record{Height: h, Body: "blockAnchor", Source: src, Sequence: sm.Number, Time: t.UTC().Format(time.RFC3339)}
				if sm.Message != nil {
					r.Hash = "<sub:" + sm.Message.Type().String() + ">"
				}
				_ = enc.Encode(r)
				emitted++
			}
		}
	}
	return emitted
}

// signerURLs returns the unique signer URLs from a list of envelope
// signatures. Includes the page/identity that signed and any
// delegator chain.
func signerURLs(sigs []protocol.Signature) []string {
	if len(sigs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(sigs))
	var out []string
	add := func(u *url.URL) {
		if u == nil {
			return
		}
		s := u.String()
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, sig := range sigs {
		// Walk delegated signatures down to the leaf.
		cur := sig
		for {
			if d, ok := cur.(*protocol.DelegatedSignature); ok && d.Signature != nil {
				if d.Delegator != nil {
					add(d.Delegator)
				}
				cur = d.Signature
				continue
			}
			break
		}
		add(cur.GetSigner())
	}
	return out
}

func emit3(enc *json.Encoder, h int64, t time.Time, txn *protocol.Transaction, src string, seq uint64, signers []string) {
	if txn == nil {
		return
	}
	r := &record{
		Height:   h,
		Time:     t.UTC().Format(time.RFC3339),
		Source:   src,
		Sequence: seq,
		Signers:  signers,
	}
	// Hash: the canonical Accumulate transaction hash, so records
	// match main-chain entries directly.
	r.Hash = hex.EncodeToString(txn.GetHash())
	if txn.Body != nil {
		r.Body = txn.Body.Type().String()
	}
	if txn.Header.Principal != nil {
		r.Principal = txn.Header.Principal.String()
	}
	r.Touches = secondaryAccounts(txn)
	_ = enc.Encode(r)
}

// secondaryAccounts returns URLs of accounts that this txn body
// references besides the principal — recipients, parent books,
// authorities, etc. Best-effort, switch over the well-known body
// types that target a secondary account.
func secondaryAccounts(txn *protocol.Transaction) []string {
	if txn == nil || txn.Body == nil {
		return nil
	}
	urls := []*url.URL{}
	add := func(u *url.URL) {
		if u != nil {
			urls = append(urls, u)
		}
	}
	switch b := txn.Body.(type) {
	case *protocol.SendTokens:
		for _, to := range b.To {
			add(to.Url)
		}
	case *protocol.AddCredits:
		add(b.Recipient)
	case *protocol.IssueTokens:
		for _, to := range b.To {
			add(to.Url)
		}
	case *protocol.BurnTokens:
		// burns from principal; no secondary
	case *protocol.CreateIdentity:
		add(b.Url)
	case *protocol.CreateDataAccount:
		add(b.Url)
	case *protocol.CreateTokenAccount:
		add(b.Url)
	case *protocol.CreateToken:
		add(b.Url)
	case *protocol.CreateKeyBook:
		add(b.Url)
	case *protocol.CreateKeyPage:
		// page url is parent book + index; we'd need book context to derive
	case *protocol.UpdateAccountAuth:
		for _, op := range b.Operations {
			switch o := op.(type) {
			case *protocol.AddAccountAuthorityOperation:
				add(o.Authority)
			case *protocol.RemoveAccountAuthorityOperation:
				add(o.Authority)
			}
		}
	case *protocol.SyntheticDepositTokens:
		add(b.Source())
	case *protocol.SyntheticDepositCredits:
		add(b.Source())
	case *protocol.SyntheticBurnTokens:
		add(b.Source())
	case *protocol.SyntheticCreateIdentity:
		for _, a := range b.Accounts {
			add(a.GetUrl())
		}
	case *protocol.SyntheticWriteData:
		add(b.Source())
	case *protocol.WriteData, *protocol.WriteDataTo:
		// no secondary
	}
	if len(urls) == 0 {
		return nil
	}
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		out = append(out, u.String())
	}
	return out
}

func must(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

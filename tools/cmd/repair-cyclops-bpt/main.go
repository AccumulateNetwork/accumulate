// repair-cyclops-bpt builds (and optionally submits) the dirty-mark
// transactions that refresh the 21 stale BPT leaves on the Cyclops
// mainnet BVN. The list of accounts is the output of `snap-bpt-stale`
// against the live follower DB at BVN BPT root
// 4364ea01d2e7092a202729d68f7740973b592dee8529e729b4b17fa84a88e5d7.
//
// Modes:
//   --pretend (default)
//       Builds every envelope locally, prints one OK/FAIL line per
//       account, and exits non-zero if any envelope fails to build.
//       Optionally calls Validate against an --endpoint to check that
//       the network would accept the envelope, without submitting it.
//
//   --submit
//       Calls Submit for each envelope. Requires --endpoint, and a
//       --signer-key (ed25519 private key, hex-encoded 64 bytes) for
//       the signer URL configured per account class.
//
// The signer for the repair envelopes is configured via --signer-url
// and --signer-key. For lite-account repairs (LiteIdentity,
// LiteTokenAccount, LiteDataAccount) the same external sender works
// for all of them. For ADI repairs the signer must control the ADI's
// own keypage (we sign as principal). The default signer-url is the
// lite identity derived from --signer-key, which is sufficient for the
// lite-class repairs in pretend mode.
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type RepairClass int

const (
	ClassADI RepairClass = iota
	ClassLiteIdentity
	ClassLiteTokenAccount
	ClassLiteDataAccountLive
	ClassLiteDataAccountOrphan
	ClassOrphanADI
	ClassBlockLedgerOrphan
)

func (c RepairClass) String() string {
	switch c {
	case ClassADI:
		return "ADI"
	case ClassLiteIdentity:
		return "LiteIdentity"
	case ClassLiteTokenAccount:
		return "LiteTokenAccount"
	case ClassLiteDataAccountLive:
		return "LiteDataAccount(live)"
	case ClassLiteDataAccountOrphan:
		return "LiteDataAccount(orphan)"
	case ClassOrphanADI:
		return "ADI(orphan)"
	case ClassBlockLedgerOrphan:
		return "BlockLedger(orphan)"
	}
	return "?"
}

type Target struct {
	URL   string
	Class RepairClass
}

// Targets is the canonical list, derived from
// /tmp/cyclops-bvn-stale-final.log. Block-ledger orphan
// (bvn-Cyclops.acme/ledger/960446) is excluded — block ledgers are
// pruned routinely and their stale BPT leaves are expected.
var Targets = []Target{
	// 7 ADIs (live, with body)
	{"acc://aber.acme", ClassADI},
	{"acc://chimneypiece.acme", ClassADI},
	{"acc://corrupted.acme", ClassADI},
	{"acc://csrc.acme", ClassADI},
	{"acc://nadro.acme", ClassADI},
	{"acc://treble.acme", ClassADI},
	{"acc://zagg.acme", ClassADI},

	// 4 LiteIdentity
	{"acc://45875c282cbf0265fc2369cfc420ab7658f9c378b257608f", ClassLiteIdentity},
	{"acc://981fabf9e5447ead08f2bb1dd7eed3282864ad20a7fc0e1e", ClassLiteIdentity},
	{"acc://ca6c6f2b20ac4fe16cf0e2a6dd1e6d8ccfce21df3fe22468", ClassLiteIdentity},
	{"acc://cb5a976eea2b84a9c78263984bc4ebf205ce99e2d2bfea01", ClassLiteIdentity},

	// 3 LiteTokenAccount
	{"acc://1570f386a1cd332a5a33beee62b0dd23df2a08bb74d23f1e/PegNet.acme/assets/peg", ClassLiteTokenAccount},
	{"acc://78432e204c43d61286daa3800cf462f8a02d8828fdd294b3/ACME", ClassLiteTokenAccount},
	{"acc://ca59e9e6c08ed245324f4c52e61defe34ab95a15abdcc802/PegNet.acme/assets/rvn", ClassLiteTokenAccount},

	// 3 LiteDataAccount (live, with body)
	{"acc://2d7b3f44935ee7de9e99766f995aa4afbc3bb9ff3dfebd9aaa8e670f178bc83c", ClassLiteDataAccountLive},
	{"acc://79f09991516f7b88c507c554bc13aa659d9bfff54467a0a4a4372f3468e88bd8", ClassLiteDataAccountLive},
	{"acc://c2db482c10bfa53099a06555d3dc5307076138a8bd003757b18f8d9c181a41c6", ClassLiteDataAccountLive},

	// 3 LiteDataAccount (orphan: body deleted)
	{"acc://675f6bdb8c270e4a573970c69c5ebac7a86c7b3dfa47b41e0fd522ab16654d55", ClassLiteDataAccountOrphan},
	{"acc://99a480cecc61722afebe6de6874ed42c2a847ff413ffd5f0f1e2243d27b630d8", ClassLiteDataAccountOrphan},
	{"acc://ab7a5ed9d394389090a62fa199b9b76624de9f2ef6c77fae77b2e8fa8723fdef", ClassLiteDataAccountOrphan},

	// 1 orphan ADI (body deleted)
	{"acc://kmutt.acme", ClassOrphanADI},
}

type Options struct {
	Pretend     bool
	Endpoint    string
	SignerURL   string
	SignerKey   ed25519.PrivateKey
	WaitTimeout time.Duration
}

var flags struct {
	Pretend   bool
	Submit    bool
	Endpoint  string
	SignerURL string
	SignerKey string
	Wait      time.Duration
}

func main() {
	cmd := &cobra.Command{
		Use:   "repair-cyclops-bpt",
		Short: "Build (and optionally submit) repair envelopes for the 21 stale BPT leaves on Cyclops",
		RunE:  run,
	}
	cmd.Flags().BoolVar(&flags.Pretend, "pretend", true, "build envelopes but do not submit (default true; use --submit to send)")
	cmd.Flags().BoolVar(&flags.Submit, "submit", false, "actually send the envelopes (requires --endpoint and --signer-key)")
	cmd.Flags().StringVar(&flags.Endpoint, "endpoint", "", "v3 JSON-RPC endpoint (e.g. https://mainnet.accumulatenetwork.io/v3)")
	cmd.Flags().StringVar(&flags.SignerURL, "signer-url", "", "URL of the signer (default: lite identity derived from --signer-key)")
	cmd.Flags().StringVar(&flags.SignerKey, "signer-key", "", "ed25519 private key, hex-encoded 64 bytes")
	cmd.Flags().DurationVar(&flags.Wait, "wait", 60*time.Second, "max time to wait per envelope (submit mode)")

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(_ *cobra.Command, _ []string) error {
	opts := Options{
		Pretend:     !flags.Submit,
		Endpoint:    flags.Endpoint,
		SignerURL:   flags.SignerURL,
		WaitTimeout: flags.Wait,
	}

	// Resolve signer key. For pretend mode without an explicit key,
	// generate a throwaway one — we just need the build path to
	// produce a well-formed envelope.
	if flags.SignerKey != "" {
		raw, err := hex.DecodeString(strings.TrimPrefix(flags.SignerKey, "0x"))
		if err != nil {
			return fmt.Errorf("decode signer key: %w", err)
		}
		switch len(raw) {
		case ed25519.PrivateKeySize:
			opts.SignerKey = raw
		case ed25519.SeedSize:
			opts.SignerKey = ed25519.NewKeyFromSeed(raw)
		default:
			return fmt.Errorf("signer key must be 32-byte seed or 64-byte private key, got %d bytes", len(raw))
		}
	} else if opts.Pretend {
		_, opts.SignerKey, _ = ed25519.GenerateKey(nil)
	} else {
		return fmt.Errorf("--signer-key is required for --submit")
	}

	if opts.SignerURL == "" {
		opts.SignerURL = protocol.LiteAuthorityForKey(opts.SignerKey[32:], protocol.SignatureTypeED25519).String()
	}

	if !opts.Pretend && opts.Endpoint == "" {
		return fmt.Errorf("--endpoint is required for --submit")
	}

	var client *jsonrpc.Client
	if opts.Endpoint != "" {
		client = jsonrpc.NewClient(opts.Endpoint)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var built, failed int
	for i, t := range Targets {
		env, err := BuildRepair(t, opts)
		if err != nil {
			fmt.Printf("[%2d/%d] FAIL build  %-12s %s: %v\n", i+1, len(Targets), t.Class, t.URL, err)
			failed++
			continue
		}
		built++

		switch {
		case opts.Pretend && client != nil:
			if _, err := client.Validate(ctx, env, api.ValidateOptions{}); err != nil {
				fmt.Printf("[%2d/%d] FAIL valid  %-12s %s: %v\n", i+1, len(Targets), t.Class, t.URL, err)
				failed++
				continue
			}
			fmt.Printf("[%2d/%d] OK    valid %-12s %s\n", i+1, len(Targets), t.Class, t.URL)

		case opts.Pretend:
			fmt.Printf("[%2d/%d] OK    parse %-12s %s  (txn=%v sigs=%d)\n",
				i+1, len(Targets), t.Class, t.URL, env.Transaction[0].Body.Type(), len(env.Signatures))

		default:
			if _, err := client.Submit(ctx, env, api.SubmitOptions{}); err != nil {
				fmt.Printf("[%2d/%d] FAIL submit %-12s %s: %v\n", i+1, len(Targets), t.Class, t.URL, err)
				failed++
				continue
			}
			fmt.Printf("[%2d/%d] OK    submit %-12s %s\n", i+1, len(Targets), t.Class, t.URL)
		}
	}

	fmt.Println()
	fmt.Printf("built %d/%d  failed %d\n", built, len(Targets), failed)
	if failed > 0 {
		return errors.New("one or more envelopes failed")
	}
	return nil
}

// BuildRepair constructs the dirty-mark envelope for a single target.
// Each repair class uses the txn shape demonstrated to refresh the BPT
// leaf in test/e2e/bpt_repair_test.go.
func BuildRepair(t Target, opts Options) (*messaging.Envelope, error) {
	target, err := url.Parse(t.URL)
	if err != nil {
		return nil, fmt.Errorf("parse URL %q: %w", t.URL, err)
	}
	signerUrl, err := url.Parse(opts.SignerURL)
	if err != nil {
		return nil, fmt.Errorf("parse signer URL: %w", err)
	}
	ts := time.Now().UnixMilli()

	switch t.Class {
	case ClassADI, ClassOrphanADI:
		// Principal = the ADI itself; create a sub-account under it
		// so the principal is marked dirty. Requires the ADI's own
		// signer; in pretend mode we sign with the throwaway key as
		// the ADI's book/1.
		bookSigner := target.JoinPath("book", "1")
		return build.Transaction().
			For(target).
			CreateDataAccount(target.JoinPath("repair-bpt-leaf")).
			SignWith(bookSigner).Version(1).Timestamp(ts).PrivateKey(opts.SignerKey).
			Done()

	case ClassLiteIdentity:
		// AddCredits to the lite identity. Sender is our own lite
		// token account; recipient (and dirtied account) is the
		// target lite identity.
		senderLTA := signerUrl.JoinPath(protocol.ACME)
		return build.Transaction().
			For(senderLTA).
			AddCredits().To(target).Spend(0.001).WithOracle(0.05).
			SignWith(signerUrl).Version(1).Timestamp(ts).PrivateKey(opts.SignerKey).
			Done()

	case ClassLiteTokenAccount:
		// SendTokens to the LTA. The sender must hold the same
		// token. For ACME/PegNet this depends on operator wallet
		// state; the pretend path just verifies the envelope builds.
		senderLTA := signerUrl.JoinPath(strings.TrimPrefix(target.Path, "/"))
		return build.Transaction().
			For(senderLTA).
			SendTokens(big.NewInt(1), 0).To(target).
			SignWith(signerUrl).Version(1).Timestamp(ts).PrivateKey(opts.SignerKey).
			Done()

	case ClassLiteDataAccountLive, ClassLiteDataAccountOrphan:
		// WriteData to the LDA. Anyone can write; for orphans the
		// write recreates the body and refreshes the leaf.
		return build.Transaction().
			For(target).
			Body(&protocol.WriteData{
				Entry: &protocol.DoubleHashDataEntry{
					Data: [][]byte{[]byte("bpt-leaf-refresh")},
				},
			}).
			SignWith(signerUrl).Version(1).Timestamp(ts).PrivateKey(opts.SignerKey).
			Done()

	case ClassBlockLedgerOrphan:
		return nil, fmt.Errorf("block-ledger orphans are not repaired by this tool")
	}
	return nil, fmt.Errorf("unknown class %v", t.Class)
}

package e2e

import (
	"bytes"
	"encoding/hex"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestEIP712Signature(t *testing.T) {
	alice := build.
		Identity("alice").Create("book").
		Tokens("tokens").Create("ACME").Add(1e9).Identity().
		Book("book").Page(1).Create().AddCredits(1e9).Book().Identity()
	aliceKey := alice.Book("book").Page(1).
		GenerateKey(SignatureTypeETH)

	t.Run("Correct chain", func(t *testing.T) {
		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork("DevNet", 3, 1),
			simulator.Genesis(GenesisTime).With(alice),
		)

		// Build a transaction
		txn, err := build.Transaction().For(alice, "book", "1").
			BurnCredits(1).
			Done()
		require.NoError(t, err)

		// Sign it
		pk, _ := aliceKey.GetPublicKey()
		sig := &TypedDataSignature{
			ChainID:       EthChainID("DevNet"),
			Signer:        alice.Url().JoinPath("book", "1"),
			PublicKey:     pk,
			SignerVersion: 1,
			Timestamp:     1,
		}
		txn.Header.Initiator = [32]byte(sig.Metadata().Hash())
		sk, _ := aliceKey.GetPrivateKey()
		require.NoError(t, SignEip712TypedData(sig, sk, nil, txn))

		// Submit
		st := sim.SubmitSuccessfully(&messaging.Envelope{
			Signatures:  []Signature{sig},
			Transaction: []*Transaction{txn},
		})

		sim.StepUntil(
			Txn(st[0].TxID).Completes())
	})

	t.Run("Incorrect chain", func(t *testing.T) {
		// Initialize
		sim := NewSim(t,
			simulator.SimpleNetwork("DevNet", 3, 1),
			simulator.Genesis(GenesisTime).With(alice),
		)

		// Build a transaction
		txn, err := build.Transaction().For(alice, "book", "1").
			BurnCredits(1).
			Done()
		require.NoError(t, err)

		// Sign it
		pk, _ := aliceKey.GetPublicKey()
		sig := &TypedDataSignature{
			ChainID:       EthChainID("MainNet"),
			Signer:        alice.Url().JoinPath("book", "1"),
			PublicKey:     pk,
			SignerVersion: 1,
			Timestamp:     1,
		}
		txn.Header.Initiator = [32]byte(sig.Metadata().Hash())
		sk, _ := aliceKey.GetPrivateKey()
		require.NoError(t, SignEip712TypedData(sig, sk, nil, txn))

		// Submit
		st := sim.Submit(&messaging.Envelope{
			Signatures:  []Signature{sig},
			Transaction: []*Transaction{txn},
		})
		require.EqualError(t, st[1].AsError(), "invalid chain ID: want 317849881, got 281")
	})
}

func TestEIP712ExternalWallet(t *testing.T) {
	acctesting.SkipWithoutTool(t, "node")

	alice := build.
		Identity("alice").Create("book").
		Tokens("tokens").Create("ACME").Add(1e9).Identity().
		Book("book").Page(1).Create().AddCredits(1e9).Book().Identity()
	aliceKey := alice.Book("book").Page(1).
		GenerateKey(SignatureTypeETH)

	// Initialize
	sim := NewSim(t,
		simulator.SimpleNetwork("DevNet", 3, 1),
		simulator.Genesis(GenesisTime).With(alice),
	)

	// Build a transaction
	txn, err := build.Transaction().For(alice, "book", "1").
		BurnCredits(1).
		Done()
	require.NoError(t, err)

	// Sign it
	pk, _ := aliceKey.GetPublicKey()
	sig := &TypedDataSignature{
		ChainID:       EthChainID("DevNet"),
		Signer:        alice.Url().JoinPath("book", "1"),
		PublicKey:     pk,
		SignerVersion: 1,
		Timestamp:     1,
	}
	txn.Header.Initiator = [32]byte(sig.Metadata().Hash())

	b, err := MarshalEip712(txn, sig)
	require.NoError(t, err)
	sk, _ := aliceKey.GetPrivateKey()

	cmd := exec.Command("../cmd/eth_signTypedData/execute.sh", hex.EncodeToString(sk), string(b))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	require.NoError(t, err)

	out = bytes.TrimSpace(out)
	out = bytes.TrimPrefix(out, []byte("0x"))
	sig.Signature = make([]byte, len(out)/2)
	_, err = hex.Decode(sig.Signature, out)
	require.NoError(t, err)

	// Submit
	st := sim.SubmitSuccessfully(&messaging.Envelope{
		Signatures:  []Signature{sig},
		Transaction: []*Transaction{txn},
	})

	sim.StepUntil(
		Txn(st[0].TxID).Completes())
}

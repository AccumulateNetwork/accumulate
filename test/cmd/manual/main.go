package main

import (
	"context"
	"encoding/hex"
	"fmt"
	stdurl "net/url"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	btc "github.com/btcsuite/btcd/btcec"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/exp/apiutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	accapi "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	accrpc "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	wapi "gitlab.com/accumulatenetwork/core/wallet/cmd/accumulate/walletd/api"
	"gitlab.com/accumulatenetwork/core/wallet/pkg/interactive"
	"gitlab.com/accumulatenetwork/core/wallet/pkg/wallet"
	wrpc "gitlab.com/accumulatenetwork/core/wallet/pkg/wallet/jsonrpc"
)

func main() {
	_ = cmd.Execute()
}

var cmd = &cobra.Command{
	Use:  "pending-gc [account]",
	Args: cobra.ExactArgs(1),
	Run:  run,
}

var flag = struct {
	Network string
	Daemon  string
	Wallet  string
	Vault   string
}{}

func init() {
	cu, err := user.Current()
	check(err)
	cmd.Flags().StringVarP(&flag.Network, "network", "n", "kermit", "The network to send the transaction to")
	cmd.Flags().StringVarP(&flag.Daemon, "daemon", "d", "unix://"+filepath.Join(cu.HomeDir, ".accumulate", "wallet", "daemon.socket"), "The wallet daemon")
	cmd.Flags().StringVarP(&flag.Wallet, "wallet", "w", filepath.Join(cu.HomeDir, ".accumulate", "wallet"), "The wallet path")
	cmd.Flags().StringVarP(&flag.Vault, "vault", "v", "", "The wallet vault")
}

func run(_ *cobra.Command, args []string) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	network := accrpc.NewClient(accumulate.ResolveWellKnownEndpoint(flag.Network, "v3"))

	// Connect to the wallet
	u, err := stdurl.Parse(flag.Daemon)
	check(err)
	var tr wrpc.Transport = wrpc.NetDialTransport(u.Scheme, u.Path)
	wallet, err := wrpc.NewWith("http://socket/wallet", tr)
	check(err)

	// Get an auth token
	token, err := wallet.RefreshToken(context.Background(), &wapi.RefreshTokenRequest{})
	check(err)

	// Autopopulate fields
	tr = &wrpc.AutoWalletTransport{
		Transport: tr,
		SetWallet: func(_ any, v *string) { *v = flag.Wallet },
		SetVault:  func(_ any, v *string) { *v = flag.Vault },
		SetToken:  func(_ any, v *[]byte) { *v = token.Token },
	}

	// Handle authentication
	it := new(wrpc.InteractiveAuthnTransport)
	it.Transport = tr
	it.Use1Password = true
	it.GetPassword = interactive.AuthenticateVault

	wallet.Transport = it

	// Find a key
	signers, err := wallet.FindSigner(ctx, &wapi.FindSignerRequest{
		Authorities: []*url.URL{protocol.DnUrl().JoinPath(protocol.Operators)},
	})
	check(err)
	signReq, signKey, err := buildSigningRequestForPaths(ctx, accapi.Querier2{Querier: network}, signers.Paths)
	check(err)

	if len(signReq.Delegators) > 0 {
		fatalf("delegators are not supported")
	}

	// Route the target
	target := url.MustParse(args[0])
	ns, err := network.NetworkStatus(ctx, accapi.NetworkStatusOptions{Partition: protocol.Directory})
	check(err)
	partition, err := apiutil.RouteAccount(ns.Routing, target)
	check(err)

	// Load the operators page
	var page *protocol.KeyPage
	_, err = api.Querier2{Querier: network}.QueryAccountAs(ctx, signReq.Signer, nil, &page)
	check(err)

	// Generate the transaction
	env, err := build.Transaction().
		For(protocol.PartitionUrl(partition)).
		Body(&protocol.NetworkMaintenance{
			Operations: []protocol.NetworkMaintenanceOperation{
				&protocol.PendingTransactionGCOperation{
					Account: target,
				},
			},
		}).
		SignWith(page.Url).
		Version(page.Version).
		Timestamp(time.Now()).
		Signer(&WalletSigner{
			Wallet: wallet,
			Key: &address.PublicKey{
				Type: signKey.KeyInfo.Type,
				Key:  signReq.PublicKey,
			},
		}).
		Done()
	check(err)

	if !env.Signatures[0].(protocol.KeySignature).Verify(nil, env.Transaction[0].GetHash()) {
		fatalf("signature is invalid")
	}

	// Submit the transaction
	subs, err := network.Submit(ctx, env, accapi.SubmitOptions{})
	check(err)
	for _, sub := range subs {
		fmt.Println(sub.Status.TxID)
		check(sub.Status.AsError())
	}
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		err = errors.UnknownError.Skip(1).Wrap(err)
		fatalf("%+v", err)
	}
}

/************************************************
 * Ripped from the wallet                       *
 ************************************************/

func buildSigningRequestForPaths(ctx context.Context, Q api.Querier2, paths []*wapi.SigningPath) (*wapi.SignRequest, *wapi.Key, error) {
	// Group paths by final authority and build a list of authorities
	pathByAuth := map[[32]byte][]*wapi.SigningPath{}
	var authorities []*url.URL
	for _, path := range paths {
		book := path.Signers[len(path.Signers)-1].Identity()
		id := book.AccountID32()
		if len(pathByAuth[id]) == 0 {
			authorities = append(authorities, book)
		}
		pathByAuth[id] = append(pathByAuth[id], path)
	}

	// Choose an authority
	var authority *url.URL
	if len(authorities) == 1 {
		authority = authorities[0]
		fmt.Println("Authority:", authority)
	} else {
		i, _, err := (&promptui.Select{
			Label: "Which key book do you want to use?",
			Items: authorities,
		}).Run()
		if err != nil {
			return nil, nil, err
		}
		authority = authorities[i]
	}

	// Expand each key into a separate path
	paths = nil
	for _, path := range pathByAuth[authority.AccountID32()] {
		for _, key := range path.Keys {
			path := path.Copy()
			path.Keys = []*wapi.Key{key}
			paths = append(paths, path)
		}
	}

	// Choose a path
	var items []string
	for _, path := range paths {
		var pages []string
		for _, s := range path.Signers {
			pages = append(pages, color.GreenString(s.ShortString()))
		}
		key := hex.EncodeToString(path.Keys[0].PublicKey[:4])
		if len(path.Keys[0].Labels.Names) > 0 {
			key = path.Keys[0].Labels.Names[0]
		}
		items = append(items, fmt.Sprintf("%s @ %s", key, strings.Join(pages, color.HiBlackString(" → "))))
	}
	var path *wapi.SigningPath
	if len(paths) == 1 {
		path = paths[0]
		fmt.Println("Signer:", items[0])
	} else {
		i, _, err := (&promptui.Select{
			Label: "Which signing path do you want to use?",
			Items: items,
		}).Run()
		if err != nil {
			return nil, nil, err
		}
		path = paths[i]
	}

	// Build the request
	req, err := buildSigningRequest(ctx, Q, path.Signers[0], path.Keys[0])
	if err != nil {
		return nil, nil, err
	}

	req.Delegators = path.Signers[1:]
	return req, path.Keys[0], nil
}

func buildSigningRequest(ctx context.Context, Q api.Querier2, signerUrl *url.URL, key *wapi.Key) (*wapi.SignRequest, error) {
	// Load the signer
	var signer protocol.Signer
	_, err := Q.QueryAccountAs(ctx, signerUrl, nil, &signer)
	switch {
	case err == nil:
		// Ok

	case !errors.Is(err, errors.NotFound):
		// Unknown error
		return nil, err

	default:
		// Signer does not exist
		return nil, fmt.Errorf("signer %v does not exist (%w)", signerUrl, err)
	}

	// Set the timestamp
	timestamp := uint64(time.Now().UTC().UnixMilli())
	if _, entry, ok := signer.EntryByKey(key.PublicKey); ok && timestamp <= entry.GetLastUsedOn() {
		timestamp = entry.GetLastUsedOn() + 1
	}

	// Create the signature request
	req := new(wapi.SignRequest)
	req.Signer = signer.GetUrl()
	req.SignerVersion = signer.GetVersion()
	req.Timestamp = timestamp
	req.PublicKey = key.PublicKey
	return req, nil
}

/************************************************
 * Ripped from staking                          *
 ************************************************/

type WalletSigner struct {
	Key    *address.PublicKey
	Wallet wallet.SigningService
}

func (s *WalletSigner) SetPublicKey(sig protocol.Signature) error {
	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		sig.PublicKey = s.Key.Key

	case *protocol.ED25519Signature:
		sig.PublicKey = s.Key.Key

	case *protocol.RCD1Signature:
		sig.PublicKey = s.Key.Key

	case *protocol.BTCSignature:
		_, pubKey := btc.PrivKeyFromBytes(btc.S256(), s.Key.Key)
		sig.PublicKey = pubKey.SerializeCompressed()

	case *protocol.BTCLegacySignature:
		_, pubKey := btc.PrivKeyFromBytes(btc.S256(), s.Key.Key)
		sig.PublicKey = pubKey.SerializeUncompressed()

	case *protocol.ETHSignature:
		_, pubKey := btc.PrivKeyFromBytes(btc.S256(), s.Key.Key)
		sig.PublicKey = pubKey.SerializeUncompressed()

	default:
		return fmt.Errorf("cannot set the public key on a %T", sig)
	}

	return nil
}

func (s *WalletSigner) Sign(sig protocol.Signature, _, message []byte) error {
	// Unpack the signature
	var keySig protocol.KeySignature
	var delegators []*url.URL
	for keySig == nil {
		switch s := sig.(type) {
		case *protocol.DelegatedSignature:
			delegators = append(delegators, s.Delegator)
			sig = s.Signature
		case protocol.KeySignature:
			keySig = s
		default:
			return fmt.Errorf("unknown signature type %T", s)
		}
	}
	for i, n := 0, len(delegators); i < n/2; i++ {
		delegators[i], delegators[n-i-1] = delegators[n-i-1], delegators[i]
	}

	// Sign
	r, err := s.Wallet.Sign(context.Background(), &wapi.SignRequest{
		PublicKey:     keySig.GetPublicKey(),
		Signer:        keySig.GetSigner(),
		SignerVersion: keySig.GetSignerVersion(),
		Timestamp:     keySig.GetTimestamp(),
		Vote:          keySig.GetVote(),
		Delegators:    delegators,
		Transaction: &protocol.Transaction{
			Body: &protocol.RemoteTransaction{
				Hash: *(*[32]byte)(message),
			},
		},
	})
	if err != nil {
		return err
	}

	// Update the signature
	for s := r.Signature; ; {
		switch s := s.(type) {
		case *protocol.DelegatedSignature:
			sig = s.Signature
			continue
		case *protocol.LegacyED25519Signature:
			*keySig.(*protocol.LegacyED25519Signature) = *s
		case *protocol.ED25519Signature:
			*keySig.(*protocol.ED25519Signature) = *s
		case *protocol.RCD1Signature:
			*keySig.(*protocol.RCD1Signature) = *s
		case *protocol.BTCSignature:
			*keySig.(*protocol.BTCSignature) = *s
		case *protocol.BTCLegacySignature:
			*keySig.(*protocol.BTCLegacySignature) = *s
		case *protocol.ETHSignature:
			*keySig.(*protocol.ETHSignature) = *s
		default:
			return fmt.Errorf("unknown signature type %T", s)
		}
		return nil
	}
}

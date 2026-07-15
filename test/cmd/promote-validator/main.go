// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Command promote-validator drives on-chain follower->validator promotion on a
// running DAG-BFT network (#4058 churn-soak P3).
//
// It reads the target node's own consensus key (the [configurations.validator-key]
// from its accumulate.toml — the key run-dual actually signs headers with),
// registers that public key as an ACTIVE validator on the given partitions by
// writing an updated NetworkDefinition to dn.acme/network, signed by the DN
// operators page. The executor's WillChangeGlobals -> UpdateCommittee chain then
// bumps the committee epoch on every node and the promoted node's headers start
// getting certified.
//
// Usage:
//
//	promote-validator -server http://127.0.0.1:27720 \
//	    -promote /data/bvn1-2/accumulate.toml \
//	    -operators /data/bvn1-1/accumulate.toml,/data/bvn1-3/accumulate.toml,... \
//	    -partitions Directory,BVN1
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulated/run"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/address"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	server := flag.String("server", "", "v3 API endpoint of a live node (e.g. http://127.0.0.1:27720)")
	promote := flag.String("promote", "", "path to the accumulate.toml of the node to promote")
	operators := flag.String("operators", "", "comma-separated accumulate.toml paths providing DN operator signing keys")
	partitions := flag.String("partitions", "Directory,BVN1", "comma-separated partitions to activate the validator on")
	flag.Parse()

	if *server == "" || *promote == "" || *operators == "" {
		log.Fatal("promote-validator: -server, -promote and -operators are required")
	}

	// The public key to register = the promoted node's own validator key.
	addPub, err := validatorPubKey(*promote)
	if err != nil {
		log.Fatalf("read promote key from %s: %v", *promote, err)
	}
	log.Printf("promoting validator key %s (from %s)", hex.EncodeToString(addPub), *promote)

	// Operator signing keys (operators == node validator keys at genesis).
	var opKeys []ed25519.PrivateKey
	for _, p := range splitNonEmpty(*operators) {
		sk, err := validatorPrivKey(p)
		if err != nil {
			log.Fatalf("read operator key from %s: %v", p, err)
		}
		opKeys = append(opKeys, sk)
	}
	if len(opKeys) == 0 {
		log.Fatal("no operator keys resolved")
	}

	ctx := context.Background()
	client := jsonrpc.NewClient(accumulate.ResolveWellKnownEndpoint(*server, "v3"))

	// Pull the current network definition and mutate it.
	ns, err := client.NetworkStatus(ctx, apiv3.NetworkStatusOptions{Partition: protocol.Directory})
	if err != nil {
		log.Fatalf("query network status: %v", err)
	}
	gv := &network.GlobalValues{
		Oracle:  ns.Oracle,
		Globals: ns.Globals,
		Network: ns.Network,
		Routing: ns.Routing,
	}
	oldVersion := gv.Network.Version
	for _, part := range splitNonEmpty(*partitions) {
		gv.Network.AddValidator(addPub, part, true) // active=true => voting member
	}
	gv.Network.Version++
	log.Printf("network definition version %d -> %d; validators now %d",
		oldVersion, gv.Network.Version, len(gv.Network.Validators))

	// Build the WriteData to dn.acme/network, signed by dn.acme/operators/1.
	dnNetwork := protocol.DnUrl().JoinPath(protocol.Network)
	operPage := protocol.DnUrl().JoinPath(protocol.Operators, "1")

	sig := build.Transaction().For(dnNetwork).
		Body(&protocol.WriteData{Entry: gv.FormatNetwork(), WriteToState: true})

	// One signature per operator key; the page accepts once AcceptThreshold is met.
	var env = sig.SignWith(operPage).Version(1).Timestamp(uint64(time.Now().UnixNano())).PrivateKey(opKeys[0])
	for _, k := range opKeys[1:] {
		env = env.SignWith(operPage).Version(1).Timestamp(uint64(time.Now().UnixNano())).PrivateKey(k)
	}
	envelope, err := env.Done()
	if err != nil {
		log.Fatalf("build/sign envelope: %v", err)
	}

	subs, err := client.Submit(ctx, envelope, apiv3.SubmitOptions{})
	if err != nil {
		log.Fatalf("submit: %v", err)
	}
	for _, s := range subs {
		if s.Status != nil && s.Status.Error != nil {
			log.Fatalf("submission rejected: %v", s.Status.Error)
		}
		if !s.Success {
			log.Fatalf("submission not successful: %s", s.Message)
		}
	}
	fmt.Printf("OK submitted network-definition update: added %s active on [%s], version %d\n",
		hex.EncodeToString(addPub), *partitions, gv.Network.Version)
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// validatorAddr loads a node's accumulate.toml and returns the parsed address of
// its [configurations.validator-key].
func validatorAddr(tomlPath string) (address.Address, error) {
	if _, err := os.Stat(tomlPath); err != nil {
		return nil, err
	}
	c := new(run.Config)
	if err := c.LoadFrom(tomlPath); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	for _, cfg := range c.Configurations {
		cv, ok := cfg.(*run.CoreValidatorConfiguration)
		if !ok {
			continue
		}
		raw, ok := cv.ValidatorKey.(*run.RawPrivateKey)
		if !ok {
			return nil, fmt.Errorf("validator-key is %T, want RawPrivateKey", cv.ValidatorKey)
		}
		return address.Parse(raw.Address)
	}
	return nil, fmt.Errorf("no coreValidator configuration in %s", tomlPath)
}

func validatorPubKey(tomlPath string) ([]byte, error) {
	addr, err := validatorAddr(tomlPath)
	if err != nil {
		return nil, err
	}
	pub, ok := addr.GetPublicKey()
	if !ok {
		return nil, fmt.Errorf("no public key in validator-key")
	}
	return pub, nil
}

func validatorPrivKey(tomlPath string) (ed25519.PrivateKey, error) {
	addr, err := validatorAddr(tomlPath)
	if err != nil {
		return nil, err
	}
	sk, ok := addr.GetPrivateKey()
	if !ok {
		return nil, fmt.Errorf("no private key in validator-key")
	}
	return ed25519.PrivateKey(sk), nil
}

// Service Setup Script for AIP-50 Enforcement Model Demos
//
// This script creates a fee-service ADI with its own key pair.
// Essential for demonstrating multi-party enforcement models.
//
// Prerequisites:
//   - Devnet running on localhost:26660
//   - alice_bob_fee_examples.go has been run (lite account funded)
//
// Run: go run service_setup.go
//
// Configuration: Edit examples/aip-50/testconfig/config.go to change ADI names/version

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"time"

	cfg "gitlab.com/accumulatenetwork/accumulate/examples/aip-50/testconfig"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type ServiceCredentials struct {
	PrivateKeyHex    string `json:"privateKeyHex"`
	PublicKeyHex     string `json:"publicKeyHex"`
	PublicKeyHashHex string `json:"publicKeyHashHex"`
	ServiceADI       string `json:"serviceADI"`
	ServiceKeyBook   string `json:"serviceKeyBook"`
	ServiceKeyPage   string `json:"serviceKeyPage"`
	ServiceFeeAcct   string `json:"serviceFeeAccount"`
}

var (
	ctx    context.Context
	client *jsonrpc.Client
)

func main() {
	fmt.Println("============================================================")
	fmt.Println("AIP-50 Service Setup Script")
	fmt.Println("============================================================")
	fmt.Printf("API Endpoint: %s\n", cfg.ACC_API)
	fmt.Printf("Version: %s\n", cfg.Version)
	fmt.Printf("Service ADI: %s\n\n", cfg.ServiceADIName)

	ctx = context.Background()
	client = jsonrpc.NewClient(cfg.ACC_API)

	// Load Alice's key (used to fund the service)
	fmt.Println("Step 1: Loading Alice's key...")
	alicePrivKey := cfg.GetAliceKey()
	aliceLiteAddr := cfg.GetAliceLiteAddress()
	fmt.Printf("  Alice Lite Account: %s\n\n", aliceLiteAddr)

	// Load service key from config
	fmt.Println("Step 2: Loading service ED25519 key pair...")
	servicePrivKey := cfg.GetServiceKey()
	servicePubKey := cfg.GetServicePubKey()
	serviceKeyHash := sha256.Sum256(servicePubKey)

	fmt.Printf("  Service Private Key: %s\n", hex.EncodeToString(servicePrivKey))
	fmt.Printf("  Service Public Key:  %s\n", hex.EncodeToString(servicePubKey))
	fmt.Printf("  Service Key Hash:    %s\n\n", hex.EncodeToString(serviceKeyHash[:]))

	// Get oracle price
	fmt.Println("Step 3: Querying network oracle...")
	networkStatus, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: "BVN1"})
	check(err, "query network status")
	oracleFloat := float64(networkStatus.Oracle.Price) / protocol.AcmeOraclePrecision
	fmt.Printf("  Oracle price: %.4f\n\n", oracleFloat)

	// Create service ADI
	fmt.Printf("Step 4: Creating service ADI (%s)...\n", cfg.ServiceADIName)
	env, err := build.Transaction().
		For(aliceLiteAddr).
		CreateIdentity(cfg.ServiceADI()).
		WithKey(servicePubKey, protocol.SignatureTypeED25519).
		WithKeyBook(cfg.ServiceKeyBook()).
		SignWith(aliceLiteAddr).Version(1).Timestamp(time.Now().UTC().UnixMicro()).PrivateKey(alicePrivKey).
		Done()
	check(err, "build create service identity")
	submitAndWait(env, "create service identity")
	fmt.Println("  Waiting 10 seconds...")
	time.Sleep(10 * time.Second)

	// Add credits to service key page
	fmt.Println("Step 5: Purchasing 3000 credits to service key page...")
	env, err = build.Transaction().
		For(aliceLiteAddr).
		AddCredits().To(cfg.ServiceKeyPage()).WithOracle(oracleFloat).Purchase(3000).
		SignWith(aliceLiteAddr).Version(1).Timestamp(time.Now().UTC().UnixMicro()).PrivateKey(alicePrivKey).
		Done()
	check(err, "build add credits to service")
	submitAndWait(env, "add credits to service")
	fmt.Println("  Waiting 10 seconds...")
	time.Sleep(10 * time.Second)

	// Create service fee account
	fmt.Printf("Step 6: Creating service fee account (%s)...\n", cfg.ServiceFees())
	env, err = build.Transaction().
		For(cfg.ServiceADI()).
		CreateTokenAccount(cfg.ServiceFees()).ForToken(protocol.AcmeUrl()).WithAuthority(cfg.ServiceKeyBook()).
		SignWith(cfg.ServiceKeyPage()).Version(1).Timestamp(time.Now().UTC().UnixMicro()).PrivateKey(servicePrivKey).
		Done()
	check(err, "build create service fee account")
	submitAndWait(env, "create service fee account")
	fmt.Println("  Waiting 10 seconds...")
	time.Sleep(10 * time.Second)

	// Send some ACME to service fee account
	fmt.Println("Step 7: Sending 10 ACME to service fee account...")
	amount := big.NewInt(10 * protocol.AcmePrecision)
	env, err = build.Transaction().
		For(aliceLiteAddr).
		SendTokens(amount, protocol.AcmePrecisionPower).To(cfg.ServiceFees()).
		SignWith(aliceLiteAddr).Version(1).Timestamp(time.Now().UTC().UnixMicro()).PrivateKey(alicePrivKey).
		Done()
	check(err, "build send tokens to service")
	submitAndWait(env, "send tokens to service")
	fmt.Println("  Waiting 10 seconds...")
	time.Sleep(10 * time.Second)

	// Save credentials to file
	fmt.Println("Step 8: Saving service credentials to service_credentials.json...")
	creds := ServiceCredentials{
		PrivateKeyHex:    hex.EncodeToString(servicePrivKey),
		PublicKeyHex:     hex.EncodeToString(servicePubKey),
		PublicKeyHashHex: hex.EncodeToString(serviceKeyHash[:]),
		ServiceADI:       cfg.ServiceADI().String(),
		ServiceKeyBook:   cfg.ServiceKeyBook().String(),
		ServiceKeyPage:   cfg.ServiceKeyPage().String(),
		ServiceFeeAcct:   cfg.ServiceFees().String(),
	}
	credsJSON, _ := json.MarshalIndent(creds, "", "  ")
	err = os.WriteFile("service_credentials.json", credsJSON, 0644)
	check(err, "write service credentials")

	fmt.Println("\n============================================================")
	fmt.Println("Service Setup Complete!")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("Service Credentials (also saved to service_credentials.json):")
	fmt.Printf("  Private Key: %s\n", hex.EncodeToString(servicePrivKey))
	fmt.Printf("  Public Key:  %s\n", hex.EncodeToString(servicePubKey))
	fmt.Println()
	fmt.Println("Service Accounts:")
	fmt.Printf("  ADI:         %s\n", cfg.ServiceADI())
	fmt.Printf("  Key Book:    %s\n", cfg.ServiceKeyBook())
	fmt.Printf("  Key Page:    %s\n", cfg.ServiceKeyPage())
	fmt.Printf("  Fee Account: %s\n", cfg.ServiceFees())
	fmt.Println()
	fmt.Println("Next Steps:")
	fmt.Println("  1. Run model1_multisig_enforcement.go to demo Model 1")
	fmt.Println("  2. Run model2_namespace_enforcement.go to demo Model 2")
	fmt.Println("  3. Run model4_delegation_enforcement.go to demo Model 4")
}

func check(err error, msg string) {
	if err != nil {
		panic(fmt.Sprintf("%s: %v", msg, err))
	}
}

func submitAndWait(env *messaging.Envelope, desc string) {
	results, err := client.Submit(ctx, env, api.SubmitOptions{})
	if err != nil {
		fmt.Printf("  Error submitting %s: %v\n", desc, err)
		return
	}
	for _, r := range results {
		if r.Status == nil {
			continue
		}
		fmt.Printf("  %s: %s (code: %v)\n", desc, r.Status.TxID, r.Status.Code)
		if r.Status.Error != nil {
			fmt.Printf("    ERROR: %v\n", r.Status.Error)
		}
	}
}

package main

import (
	"fmt"
	"log"
	"time"
)

func main() {
	fmt.Println("🧪 Testing ACME Spender System")
	fmt.Println("============================")

	// Initialize ACME spender
	_, err := NewACMESpender("http://127.0.0.1:26660", &SpenderConfig{
		WorkerCount:         1,
		TransactionInterval: 5 * time.Second,
		MaxRetries:          3,
		RetryDelay:          1 * time.Second,

		// Focus on basic transaction types that work
		TokenTransferWeight: 25, // 25% transfers
		DataWriteWeight:     25, // 25% data writes
		AccountCreateWeight: 25, // 25% ADI creation
		TokenSendWeight:     25, // 25% simple sends
		// Set others to 0 for now
		DataCollectWeight:        0,
		TokenIssueWeight:         0,
		IssuedTokenMoveWeight:    0,
		TokenAccountCreateWeight: 0,
		DataAccountCreateWeight:  0,
	})
	if err != nil {
		log.Fatalf("❌ Failed to create ACME spender: %v", err)
	}

	fmt.Println("✅ ACME spender created successfully")
	fmt.Printf("📊 Transaction weights: Transfer(%d%%), Data(%d%%), ADI(%d%%), Send(%d%%)\\n",
		25, 25, 25, 25)

	fmt.Println("\\n🎯 ACME spender system is ready for testing!")
	fmt.Println("   To run a full test with the DevNet:")
	fmt.Println("   1. Start DevNet: go run ./cmd/accumulated run devnet -w .devnet-test")
	fmt.Println("   2. Run: go run collect_and_spend_test.go acme_spender.go faucet_helper.go")
}

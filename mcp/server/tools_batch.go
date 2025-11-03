package server

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/mcp/client"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// submitBatch submits multiple transactions in a single envelope
// WARNING: Each transaction is charged individually (no fee savings)
// WARNING: Transactions execute independently (no atomicity - partial failures possible)
func (s *Server) submitBatch(args map[string]interface{}) (map[string]interface{}, error) {
	// Extract transactions array
	txnsInterface, ok := args["transactions"].([]interface{})
	if !ok || len(txnsInterface) == 0 {
		return nil, fmt.Errorf("missing or empty transactions array")
	}

	// Extract private key (used to sign all transactions)
	privateKeyHex, ok := args["private_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: private_key")
	}

	// Decode private key
	if len(privateKeyHex) > 2 && privateKeyHex[:2] == "0x" {
		privateKeyHex = privateKeyHex[2:]
	}
	privateKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	privateKey := ed25519.PrivateKey(privateKeyBytes)
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key length: expected %d, got %d", ed25519.PrivateKeySize, len(privateKey))
	}

	// Extract network
	network := "mainnet"
	if n, ok := args["network"].(string); ok {
		network = n
	}

	// Build all transactions and signatures
	var transactions []*protocol.Transaction
	var signatures []protocol.Signature

	for i, txnInterface := range txnsInterface {
		txnMap, ok := txnInterface.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("transaction %d is not a valid object", i)
		}

		txnType, ok := txnMap["type"].(string)
		if !ok {
			return nil, fmt.Errorf("transaction %d missing type", i)
		}

		params, ok := txnMap["params"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("transaction %d missing params", i)
		}

		// Build transaction based on type
		txn, err := buildTransaction(txnType, params)
		if err != nil {
			return nil, fmt.Errorf("failed to build transaction %d (%s): %w", i, txnType, err)
		}

		// Create signature
		sig := &protocol.ED25519Signature{
			PublicKey: privateKey.Public().(ed25519.PublicKey),
			Signer:    txn.Header.Principal.RootIdentity(),
			Timestamp: uint64(time.Now().UnixMilli()),
		}

		// Set initiator if this is the first transaction
		if i == 0 && txn.Header.Initiator == ([32]byte{}) {
			txn.Header.Initiator = *(*[32]byte)(sig.Metadata().Hash())
		}

		// Set the transaction hash in the signature
		sig.TransactionHash = txn.ID().Hash()

		// Sign the transaction using the signing package
		sigMdHash := sig.Metadata().Hash()
		txnHash := txn.GetHash()
		err = signing.PrivateKey(privateKey).Sign(sig, sigMdHash, txnHash)
		if err != nil {
			return nil, fmt.Errorf("failed to sign transaction %d: %w", i, err)
		}

		transactions = append(transactions, txn)
		signatures = append(signatures, sig)
	}

	// Create client
	c, err := client.NewClient(network)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	defer c.Close()

	// Submit batch
	ctx := context.Background()
	hashes, err := c.SubmitEnvelope(ctx, transactions, signatures)
	if err != nil {
		return nil, fmt.Errorf("failed to submit batch: %w", err)
	}

	// Format transaction hashes
	hashStrs := make([]string, len(hashes))
	for i, hash := range hashes {
		hashStrs[i] = hex.EncodeToString(hash)
	}

	text := fmt.Sprintf("Batch submitted successfully!\n\nNetwork: %s\nTransactions: %d\n\nTransaction Hashes:\n", network, len(transactions))
	for i, hash := range hashStrs {
		text += fmt.Sprintf("  %d. %s\n", i+1, hash)
	}
	text += "\n⚠️  NOTE: Each transaction is charged individually (no fee savings)\n"
	text += "⚠️  NOTE: Transactions execute independently (partial failures possible)\n"
	text += "⚠️  Check individual transaction statuses to verify all succeeded"

	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": text,
			},
		},
	}, nil
}

// buildTransaction builds a transaction based on type and parameters
func buildTransaction(txnType string, params map[string]interface{}) (*protocol.Transaction, error) {
	switch txnType {
	case "sendTokens":
		return buildSendTokens(params)
	case "createDataAccount":
		return buildCreateDataAccount(params)
	case "writeData":
		return buildWriteData(params)
	case "createTokenAccount":
		return buildCreateTokenAccount(params)
	case "createIdentity":
		return buildCreateIdentity(params)
	default:
		return nil, fmt.Errorf("unsupported transaction type: %s", txnType)
	}
}

// buildSendTokens builds a SendTokens transaction
func buildSendTokens(params map[string]interface{}) (*protocol.Transaction, error) {
	from, ok := params["from"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: from")
	}

	to, ok := params["to"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: to")
	}

	amountStr, ok := params["amount"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: amount")
	}

	// Parse amount
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}

	// Parse URLs
	fromUrl, err := accurl.Parse(from)
	if !ok {
		return nil, fmt.Errorf("invalid from URL: %w", err)
	}

	toUrl, err := accurl.Parse(to)
	if err != nil {
		return nil, fmt.Errorf("invalid to URL: %w", err)
	}

	// Create transaction
	body := &protocol.SendTokens{
		To: []*protocol.TokenRecipient{{
			Url:    toUrl,
			Amount: *big.NewInt(amount),
		}},
	}

	return &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: fromUrl,
		},
		Body: body,
	}, nil
}

// buildCreateDataAccount builds a CreateDataAccount transaction
func buildCreateDataAccount(params map[string]interface{}) (*protocol.Transaction, error) {
	url, ok := params["url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: url")
	}

	principal, ok := params["principal"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: principal")
	}

	// Parse URLs
	accountUrl, err := accurl.Parse(url)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	principalUrl, err := accurl.Parse(principal)
	if err != nil {
		return nil, fmt.Errorf("invalid principal: %w", err)
	}

	// Parse authorities if provided
	var authorities []*accurl.URL
	if authsInterface, ok := params["authorities"].([]interface{}); ok {
		for _, authInterface := range authsInterface {
			authStr, ok := authInterface.(string)
			if !ok {
				return nil, fmt.Errorf("invalid authority URL")
			}
			authUrl, err := accurl.Parse(authStr)
			if err != nil {
				return nil, fmt.Errorf("invalid authority URL: %w", err)
			}
			authorities = append(authorities, authUrl)
		}
	}

	// Create transaction
	body := &protocol.CreateDataAccount{
		Url:         accountUrl,
		Authorities: authorities,
	}

	return &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: principalUrl,
		},
		Body: body,
	}, nil
}

// buildWriteData builds a WriteData transaction
func buildWriteData(params map[string]interface{}) (*protocol.Transaction, error) {
	account, ok := params["account"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: account")
	}

	data, ok := params["data"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: data")
	}

	// Parse account URL
	accountUrl, err := accurl.Parse(account)
	if err != nil {
		return nil, fmt.Errorf("invalid account URL: %w", err)
	}

	// Create transaction
	body := &protocol.WriteData{
		Entry: &protocol.DoubleHashDataEntry{
			Data: [][]byte{[]byte(data)},
		},
	}

	return &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: accountUrl,
		},
		Body: body,
	}, nil
}

// buildCreateTokenAccount builds a CreateTokenAccount transaction
func buildCreateTokenAccount(params map[string]interface{}) (*protocol.Transaction, error) {
	url, ok := params["url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: url")
	}

	principal, ok := params["principal"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: principal")
	}

	tokenUrl, ok := params["token_url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: token_url")
	}

	// Parse URLs
	accountUrl, err := accurl.Parse(url)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	principalUrl, err := accurl.Parse(principal)
	if err != nil {
		return nil, fmt.Errorf("invalid principal: %w", err)
	}

	tokenUrlParsed, err := accurl.Parse(tokenUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid token_url: %w", err)
	}

	// Parse authorities if provided
	var authorities []*accurl.URL
	if authsInterface, ok := params["authorities"].([]interface{}); ok {
		for _, authInterface := range authsInterface {
			authStr, ok := authInterface.(string)
			if !ok {
				return nil, fmt.Errorf("invalid authority URL")
			}
			authUrl, err := accurl.Parse(authStr)
			if err != nil {
				return nil, fmt.Errorf("invalid authority URL: %w", err)
			}
			authorities = append(authorities, authUrl)
		}
	}

	// Create transaction
	body := &protocol.CreateTokenAccount{
		Url:         accountUrl,
		TokenUrl:    tokenUrlParsed,
		Authorities: authorities,
	}

	return &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: principalUrl,
		},
		Body: body,
	}, nil
}

// buildCreateIdentity builds a CreateIdentity transaction
func buildCreateIdentity(params map[string]interface{}) (*protocol.Transaction, error) {
	url, ok := params["url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: url")
	}

	sponsor, ok := params["sponsor"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: sponsor")
	}

	publicKeyHex, ok := params["public_key"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required parameter: public_key")
	}

	// Parse URLs
	adiUrl, err := accurl.Parse(url)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	sponsorUrl, err := accurl.Parse(sponsor)
	if err != nil {
		return nil, fmt.Errorf("invalid sponsor: %w", err)
	}

	// Decode public key
	if len(publicKeyHex) > 2 && publicKeyHex[:2] == "0x" {
		publicKeyHex = publicKeyHex[2:]
	}
	publicKeyBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}

	// Create key hash
	keyHash := sha256.Sum256(publicKeyBytes)

	// Create transaction
	body := &protocol.CreateIdentity{
		Url:        adiUrl,
		KeyHash:    keyHash[:],
		KeyBookUrl: adiUrl.JoinPath("book"),
	}

	return &protocol.Transaction{
		Header: protocol.TransactionHeader{
			Principal: sponsorUrl,
		},
		Body: body,
	}, nil
}

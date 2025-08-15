package loadgen

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/loadgen/wallet"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	"gitlab.com/accumulatenetwork/accumulate/pkg/client/signing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TransactionExecutor handles the actual execution of transactions using the v2 client
type TransactionExecutor struct {
	wallet *wallet.Wallet
	client *client.Client
}

// NewTransactionExecutor creates a new transaction executor
func NewTransactionExecutor(w *wallet.Wallet, serverURL string) (*TransactionExecutor, error) {
	c, err := client.New(serverURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	
	return &TransactionExecutor{
		wallet: w,
		client: c,
	}, nil
}

// Execute processes a transaction request and submits it to the network
func (te *TransactionExecutor) Execute(ctx context.Context, req *TransactionRequest) (*TransactionResult, error) {
	// Build and execute based on transaction type
	switch req.Type {
	case TxCreateADI:
		return te.executeCreateADI(ctx, req)
	case TxCreateKeyBook:
		return te.executeCreateKeyBook(ctx, req)
	case TxCreateKeyPage:
		return te.executeCreateKeyPage(ctx, req)
	case TxUpdateKeyPage:
		return te.executeUpdateKeyPage(ctx, req)
	case TxCreateTokenAccount:
		return te.executeCreateTokenAccount(ctx, req)
	case TxCreateDataAccount:
		return te.executeCreateDataAccount(ctx, req)
	case TxCreateLiteAccount:
		return te.executeCreateLiteAccount(ctx, req)
	case TxAddCredits:
		return te.executeAddCredits(ctx, req)
	case TxSendTokensADI, TxSendTokensLite, TxSendTokensMixed:
		return te.executeSendTokens(ctx, req)
	case TxBurnTokens:
		return te.executeBurnTokens(ctx, req)
	case TxLockAccount:
		return te.executeLockAccount(ctx, req)
	case TxWriteData, TxWriteDataToLite:
		return te.executeWriteData(ctx, req)
	case TxScratchData:
		return te.executeScratchData(ctx, req)
	case TxCreateToken:
		return te.executeCreateToken(ctx, req)
	case TxIssueTokens:
		return te.executeIssueTokens(ctx, req)
	case TxUpdateTokenIssuer:
		return te.executeUpdateTokenIssuer(ctx, req)
	default:
		return &TransactionResult{
			Request: req,
			Error:   fmt.Errorf("unsupported transaction type: %s", req.Type),
		}, nil
	}
}

// Helper to create a signer from wallet key
func (te *TransactionExecutor) createSigner(key *wallet.Key) (signing.Signer, error) {
	if key == nil || len(key.PrivateKey) == 0 {
		return nil, fmt.Errorf("no private key available")
	}
	
	switch key.Type {
	case protocol.SignatureTypeED25519:
		if len(key.PrivateKey) != ed25519.PrivateKeySize {
			return nil, fmt.Errorf("invalid ED25519 private key size")
		}
		privKey := ed25519.PrivateKey(key.PrivateKey)
		return &signing.ED25519Signer{PrivateKey: privKey}, nil
	default:
		return nil, fmt.Errorf("unsupported key type: %v", key.Type)
	}
}

// Helper to build TxRequest
func (te *TransactionExecutor) buildTxRequest(origin *url.URL, signerURL *url.URL, key *wallet.Key, payload interface{}) (*api.TxRequest, error) {
	signer, err := te.createSigner(key)
	if err != nil {
		return nil, err
	}
	
	// Create signing builder
	builder := &signing.Builder{
		Type:   key.Type,
		Url:    signerURL,
		Signer: signer,
	}
	
	// Build the request
	txReq := &api.TxRequest{
		Origin:  origin,
		Signer:  api.Signer{PublicKey: key.PublicKey, Url: signerURL},
		Payload: payload,
	}
	
	// Sign the transaction
	// Note: The v2 client handles the actual signing internally
	_ = builder
	
	return txReq, nil
}

// Transaction execution methods

func (te *TransactionExecutor) executeCreateADI(ctx context.Context, req *TransactionRequest) (*TransactionResult, error) {
	// For now, generate simple test data
	// TODO: Get from request payload
	adiName := fmt.Sprintf("adi-%d", req.CreatedAt.Unix())
	key := te.wallet.GetAllKeys()[0] // Get first available key
	
	payload := &protocol.CreateIdentity{
		Url:        url.MustParse("acc://" + adiName),
		KeyHash:    key.PublicKeyHash,
		KeyBookUrl: url.MustParse("acc://" + adiName + "/book0"),
	}
	
	txReq, err := te.buildTxRequest(payload.Url, payload.Url, key, payload)
	if err != nil {
		return &TransactionResult{Request: req, Error: err}, nil
	}
	
	resp, err := te.client.ExecuteCreateIdentity(ctx, txReq)
	if err != nil {
		return &TransactionResult{Request: req, Error: err}, nil
	}
	
	return &TransactionResult{
		Request: req,
		Success: true,
		TxID:    resp.TransactionHash,
	}, nil
}

func (te *TransactionExecutor) executeCreateKeyBook(ctx context.Context, req *TransactionRequest) (*TransactionResult, error) {
	// TODO: Implement
	return &TransactionResult{Request: req, Error: fmt.Errorf("not implemented")}, nil
}

func (te *TransactionExecutor) executeCreateKeyPage(ctx context.Context, req *TransactionRequest) (*TransactionResult, error) {
	// TODO: Implement
	return &TransactionResult{Request: req, Error: fmt.Errorf("not implemented")}, nil
}

func (te *TransactionExecutor) executeUpdateKeyPage(ctx context.Context, req *TransactionRequest) (*TransactionResult, error) {
	// TODO: Implement
	return &TransactionResult{Request: req, Error: fmt.Errorf("not implemented")}, nil
}

func (te *TransactionExecutor) executeCreateTokenAccount(ctx context.Context, req *TransactionRequest) (*TransactionResult, error) {
	// TODO: Implement properly with payload
	key := te.wallet.GetAllKeys()[0]
	
	payload := &protocol.CreateTokenAccount{
		Url:      url.MustParse("acc://test/tokens"),
		TokenUrl: protocol.AcmeUrl(),
	}
	
	txReq, err := te.buildTxRequest(payload.Url, payload.Url, key, payload)
	if err != nil {
		return &TransactionResult{Request: req, Error: err}, nil
	}
	
	resp, err := te.client.ExecuteCreateTokenAccount(ctx, txReq)
	if err != nil {
		return &TransactionResult{Request: req, Error: err}, nil
	}
	
	return &TransactionResult{
		Request: req,
		Success: true,
		TxID:    resp.TransactionHash,
	}, nil
}

func (te *TransactionExecutor) executeCreateDataAccount(ctx context.Context, req *TransactionRequest) (*TransactionResult, error) {
	// TODO: Implement
	return &TransactionResult{Request: req, Error: fmt.Errorf("not implemented")}, nil
}

func (te *TransactionExecutor) executeCreateLiteAccount(ctx context.Context, req *TransactionRequest) (*TransactionResult, error) {
	// Lite accounts are created by sending tokens to them
	// TODO: Implement via send tokens
	return &TransactionResult{Request: req, Error: fmt.Errorf("not implemented")}, nil
}

func (te *TransactionExecutor) executeAddCredits(ctx context.Context, req *TransactionRequest) (*TransactionResult, error) {
	// TODO: Implement properly with payload
	key := te.wallet.GetAllKeys()[0]
	
	payload := &protocol.AddCredits{
		Recipient: url.MustParse("acc://test/page"),
		Amount:    *big.NewInt(1000),
	}
	
	txReq, err := te.buildTxRequest(payload.Recipient, payload.Recipient, key, payload)
	if err != nil {
		return &TransactionResult{Request: req, Error: err}, nil
	}
	
	resp, err := te.client.ExecuteAddCredits(ctx, txReq)
	if err != nil {
		return &TransactionResult{Request: req, Error: err}, nil
	}
	
	return &TransactionResult{
		Request: req,
		Success: true,
		TxID:    resp.TransactionHash,
	}, nil
}

func (te *TransactionExecutor) executeSendTokens(ctx context.Context, req *TransactionRequest) (*TransactionResult, error) {
	// TODO: Implement properly with payload
	key := te.wallet.GetAllKeys()[0]
	
	payload := &protocol.SendTokens{
		To: []*protocol.TokenRecipient{
			{
				Url:    url.MustParse("acc://test/tokens2"),
				Amount: *big.NewInt(100),
			},
		},
	}
	
	from := url.MustParse("acc://test/tokens")
	txReq, err := te.buildTxRequest(from, from, key, payload)
	if err != nil {
		return &TransactionResult{Request: req, Error: err}, nil
	}
	
	resp, err := te.client.ExecuteSendTokens(ctx, txReq)
	if err != nil {
		return &TransactionResult{Request: req, Error: err}, nil
	}
	
	return &TransactionResult{
		Request: req,
		Success: true,
		TxID:    resp.TransactionHash,
	}, nil
}

func (te *TransactionExecutor) executeBurnTokens(ctx context.Context, req *TransactionRequest) (*TransactionResult, error) {
	// TODO: Implement
	return &TransactionResult{Request: req, Error: fmt.Errorf("not implemented")}, nil
}

func (te *TransactionExecutor) executeLockAccount(ctx context.Context, req *TransactionRequest) (*TransactionResult, error) {
	// TODO: Implement
	return &TransactionResult{Request: req, Error: fmt.Errorf("not implemented")}, nil
}

func (te *TransactionExecutor) executeWriteData(ctx context.Context, req *TransactionRequest) (*TransactionResult, error) {
	// TODO: Implement properly with payload
	key := te.wallet.GetAllKeys()[0]
	
	payload := &protocol.WriteData{
		Entry: &protocol.DataEntry{
			Data: []byte("test data"),
		},
	}
	
	account := url.MustParse("acc://test/data")
	txReq, err := te.buildTxRequest(account, account, key, payload)
	if err != nil {
		return &TransactionResult{Request: req, Error: err}, nil
	}
	
	resp, err := te.client.ExecuteWriteData(ctx, txReq)
	if err != nil {
		return &TransactionResult{Request: req, Error: err}, nil
	}
	
	return &TransactionResult{
		Request: req,
		Success: true,
		TxID:    resp.TransactionHash,
	}, nil
}

func (te *TransactionExecutor) executeScratchData(ctx context.Context, req *TransactionRequest) (*TransactionResult, error) {
	// TODO: Implement
	return &TransactionResult{Request: req, Error: fmt.Errorf("not implemented")}, nil
}

func (te *TransactionExecutor) executeCreateToken(ctx context.Context, req *TransactionRequest) (*TransactionResult, error) {
	// TODO: Implement
	return &TransactionResult{Request: req, Error: fmt.Errorf("not implemented")}, nil
}

func (te *TransactionExecutor) executeIssueTokens(ctx context.Context, req *TransactionRequest) (*TransactionResult, error) {
	// TODO: Implement
	return &TransactionResult{Request: req, Error: fmt.Errorf("not implemented")}, nil
}

func (te *TransactionExecutor) executeUpdateTokenIssuer(ctx context.Context, req *TransactionRequest) (*TransactionResult, error) {
	// TODO: Implement
	return &TransactionResult{Request: req, Error: fmt.Errorf("not implemented")}, nil
}

// ED25519Signer implements the signing.Signer interface
type ED25519Signer struct {
	PrivateKey ed25519.PrivateKey
}

func (s *ED25519Signer) Sign(sig protocol.Signature, sigHash, hash []byte) error {
	switch sig := sig.(type) {
	case *protocol.LegacyED25519Signature:
		sig.Signature = ed25519.Sign(s.PrivateKey, hash)
		sig.PublicKey = s.PrivateKey.Public().(ed25519.PublicKey)
	case *protocol.ED25519Signature:
		sig.Signature = ed25519.Sign(s.PrivateKey, hash)
		sig.PublicKey = s.PrivateKey.Public().(ed25519.PublicKey)
	default:
		return fmt.Errorf("unsupported signature type: %T", sig)
	}
	return nil
}

func (s *ED25519Signer) PublicKey() []byte {
	return s.PrivateKey.Public().(ed25519.PublicKey)
}

func (s *ED25519Signer) PublicKeyHash() []byte {
	hash := sha256.Sum256(s.PublicKey())
	return hash[:]
}
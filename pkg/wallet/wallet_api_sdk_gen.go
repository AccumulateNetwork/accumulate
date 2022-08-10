package wallet

// GENERATED BY go run ./tools/cmd/gen-sdk. DO NOT EDIT.

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/cmd/accumulate/walletd/api"
)

// AdiList returns a list of adi's managed by the wallet.
func (c *Client) AdiList(ctx context.Context) (interface{}, error) {
	var req struct{}
	var resp interface{}

	err := c.RequestAPIv2(ctx, "adi-list", req, &resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// CreateEnvelope create an envelope by name.
func (c *Client) CreateEnvelope(ctx context.Context, req *api.CreateEnvelopeRequest) (*api.CreateEnvelopeResponse, error) {
	var resp api.CreateEnvelopeResponse

	err := c.RequestAPIv2(ctx, "create-envelope", req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// CreateTransaction create a transaction by name.
func (c *Client) CreateTransaction(ctx context.Context, req *api.CreateTransactionRequest) (*api.CreateTransactionResponse, error) {
	var resp api.CreateTransactionResponse

	err := c.RequestAPIv2(ctx, "create-transaction", req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// Decode unmarshal a binary transaction or account and return the json transaction body.
func (c *Client) Decode(ctx context.Context, req *api.DecodeRequest) (*api.DecodeResponse, error) {
	var resp api.DecodeResponse

	err := c.RequestAPIv2(ctx, "decode", req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// Encode binary marshal a json transaction or account and return encoded hex.
func (c *Client) Encode(ctx context.Context, req *api.EncodeRequest) (interface{}, error) {
	var resp interface{}

	err := c.RequestAPIv2(ctx, "encode", req, &resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// KeyList returns a list of available keys in the wallet.
func (c *Client) KeyList(ctx context.Context) (interface{}, error) {
	var req struct{}
	var resp interface{}

	err := c.RequestAPIv2(ctx, "key-list", req, &resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// ResolveKey returns a public key from either a label or keyhash.
func (c *Client) ResolveKey(ctx context.Context, req *api.ResolveKeyRequest) (*api.ResolveKeyResponse, error) {
	var resp api.ResolveKeyResponse

	err := c.RequestAPIv2(ctx, "resolve-key", req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// Sign sign a transaction.
func (c *Client) Sign(ctx context.Context, req *api.SignRequest) (interface{}, error) {
	var resp interface{}

	err := c.RequestAPIv2(ctx, "sign", req, &resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// Version returns the version of the wallet daemon.
func (c *Client) Version(ctx context.Context) (*api.VersionResponse, error) {
	var req struct{}
	var resp api.VersionResponse

	err := c.RequestAPIv2(ctx, "version", req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

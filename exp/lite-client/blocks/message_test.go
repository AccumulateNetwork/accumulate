package blocks

import (
	"context"
	"fmt"
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	message "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	mocks "gitlab.com/accumulatenetwork/accumulate/test/mocks/pkg/api/v3"
)

type routerFunc func(message.Message) (multiaddr.Multiaddr, error)

func (fn routerFunc) Route(msg message.Message) (multiaddr.Multiaddr, error) { return fn(msg) }

func TestQueryMessageRecord(t *testing.T) {
	// Create a realistic mock block message with signatures and authority
	authorityUrl := url.MustParse("acc://authority")
	authority := &protocol.LiteTokenAccount{Url: authorityUrl}
	blockId := url.MustParse("acc://block").WithTxID([32]byte{0xaa, 0xbb, 0xcc})

	// Mock a signature record
	sigRecord := &api.SignatureSetRecord{
		Account: authority, // protocol.Account interface
		Signatures: &api.RecordRange[*api.MessageRecord[messaging.Message]]{
			Records: []*api.MessageRecord[messaging.Message]{
				{
					ID: url.MustParse("acc://sig1").WithTxID([32]byte{0x01}),
					Message: &messaging.SignatureMessage{
						Signature: &protocol.ED25519Signature{
							PublicKey: []byte{0x11, 0x22, 0x33},
							Signature: []byte{0x44, 0x55, 0x66},
						},
						TxID: blockId,
					},
				},
			},
		},
	}

	// Mock the block message (using SyntheticMessage as a stand-in)
	blockMsg := &messaging.SyntheticMessage{
		Message: nil, // You could nest a more realistic message here
	}

	// Main mock record for the block
	expect := &api.MessageRecord[messaging.Message]{
		ID:         blockId,
		Message:    blockMsg,
		Status:     0,
		Signatures: &api.RecordRange[*api.SignatureSetRecord]{Records: []*api.SignatureSetRecord{sigRecord}},
		Historical: false,
	}
	fmt.Printf("[Test] Expecting BlockMessage ID: %s\n", expect.ID.String())
	fmt.Printf("[Test] Expecting BlockMessage Authority: %s\n", authorityUrl.String())

	s := mocks.NewQuerier(t)
	s.EXPECT().Query(mock.Anything, mock.Anything, mock.Anything).Return(expect, nil)
	c := SetupTest(t, message.Querier{Querier: s})
	actual, err := c.Query(context.Background(), protocol.AccountUrl("block"), nil)

	actualMsgRec, ok := actual.(*api.MessageRecord[messaging.Message])
	require.True(t, ok, "expected MessageRecord type")

	fmt.Printf("[Test] Actual BlockMessage ID:   %s\n", actualMsgRec.ID.String())
	if expect.ID != nil && actualMsgRec.ID != nil {
		fmt.Printf("[Test] Expecting BlockMessage Hash: %x\n", expect.ID.Hash())
		fmt.Printf("[Test] Actual BlockMessage Hash:   %x\n", actualMsgRec.ID.Hash())
		fmt.Printf("[Test] Expecting BlockMessage Account: %s\n", expect.ID.Account())
		fmt.Printf("[Test] Actual BlockMessage Account:   %s\n", actualMsgRec.ID.Account())
	}
	fmt.Printf("[Test] Error: %v\n", err)

	require.NoError(t, err)
	// Check signatures
	if actualMsgRec.Signatures != nil && len(actualMsgRec.Signatures.Records) > 0 {
		for i, sigSet := range actualMsgRec.Signatures.Records {
			fmt.Printf("[Test] SignatureSetRecord[%d] Account: %s\n", i, sigSet.Account)
			if sigSet.Signatures != nil && len(sigSet.Signatures.Records) > 0 {
				for j, sigMsgRec := range sigSet.Signatures.Records {
					fmt.Printf("[Test] Signature[%d][%d] ID: %s\n", i, j, sigMsgRec.ID)
					if sig, ok := sigMsgRec.Message.(*messaging.SignatureMessage); ok {
						fmt.Printf("[Test] Signature[%d][%d] Type: %T\n", i, j, sig.Signature)
						if ed, ok := sig.Signature.(*protocol.ED25519Signature); ok {
							fmt.Printf("[Test] ED25519 PublicKey: %x\n", ed.PublicKey)
							fmt.Printf("[Test] ED25519 Signature: %x\n", ed.Signature)
						}
					}
				}
			}
		}
	}

	require.True(t, expect.ID.Equal(actualMsgRec.ID), "IDs should be logically equal")
	// Optionally, compare message contents as well
}

func SetupTest(t testing.TB, services ...message.Service) *message.Client {
	handler, err := message.NewHandler(services...)
	require.NoError(t, err)
	addr, err := multiaddr.NewComponent(api.N_ACC_SVC, "unknown:foo")
	require.NoError(t, err)
	return &message.Client{Transport: &message.RoutedTransport{
		Network: "foo",
		Router:  routerFunc(func(m message.Message) (multiaddr.Multiaddr, error) { return addr, nil }),
		Dialer: dialerFunc(func(ctx context.Context, _ multiaddr.Multiaddr) (message.Stream, error) {
			s := message.Pipe(ctx)
			go func() { <-ctx.Done(); s.Close() }()
			go handler.Handle(s)
			return s, nil
		}),
	}}
}

type dialerFunc func(context.Context, multiaddr.Multiaddr) (message.Stream, error)

func (fn dialerFunc) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	return fn(ctx, addr)
}

func TestQueryMessageRecord_Real(t *testing.T) {
	// Replace with the real node multiaddr you want to connect to
	nodeAddr, err := multiaddr.NewMultiaddr("/dns/localhost/tcp/16591")
	require.NoError(t, err)

	// Create a transport
	node, err := p2p.New(p2p.Options{Network: "MainNet"})
	require.NoError(t, err)

	dialer := node.DialNetwork()

	transport := &message.RoutedTransport{
		Network: "Kermit",
		Dialer:  dialer,
		Router: routerFunc(func(msg message.Message) (multiaddr.Multiaddr, error) {
			return nodeAddr, nil
		}),
	}

	// Create the client
	client := &message.Client{Transport: transport}

	// Pick a real account or record URL that exists on your node
	recordUrl, err := url.Parse("acc://some-existing-account-or-block")
	recordUrl = recordUrl.JoinPath(nodeAddr.String())
	require.NoError(t, err)

	// Query for the record (no additional query object for basic record fetch)
	rec, err := client.Query(context.Background(), recordUrl, nil)
	require.NoError(t, err)
	require.NotNil(t, rec)

	// Print the type and details of the returned record
	fmt.Printf("[Real Query] Record type: %T\n", rec)
	fmt.Printf("[Real Query] Record: %+v\n", rec)

	// If it's a MessageRecord, print deeper details
	if msgRec, ok := rec.(*api.MessageRecord[messaging.Message]); ok {
		fmt.Printf("[Real Query] MessageRecord ID: %s\n", msgRec.ID)
		fmt.Printf("[Real Query] Message: %T\n", msgRec.Message)
		if msgRec.Signatures != nil {
			fmt.Printf("[Real Query] Signatures: %+v\n", msgRec.Signatures)
		}
	}
}

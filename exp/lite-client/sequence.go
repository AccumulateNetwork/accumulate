package liteclient

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	client "gitlab.com/accumulatenetwork/accumulate/pkg/client/api/v2"
	accurl "gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// VerifyMajorBlockSequence ensures that the provided major blocks are sequentially numbered
// and that their timestamps follow the expected schedule.
// Used to check for missing or out-of-order blocks.
//
// TODO: Implement sequence and timestamp validation for major blocks.
func validateBlockSequence(ctx context.Context, cl client.Client, blocks []*client.MajorQueryResponse, partitionUrl *accurl.URL) error {
	globals, err := QueryNetworkGlobals(ctx, cl, partitionUrl)
	if err != nil {
		return fmt.Errorf("failed to query network globals: %v", err)
	}

	// Parse the cron schedule
	schedule, err := cron.ParseStandard(globals.MajorBlockSchedule)
	if err != nil {
		return fmt.Errorf("invalid major block schedule: %v", err)
	}

	if len(blocks) == 0 {
		return nil
	}
	refTime := blocks[0].MajorBlockTime
	if refTime == nil {
		return fmt.Errorf("first block has nil MajorBlockTime")
	}

	expectedTime := *refTime
	tolerance := time.Minute

	for i := 1; i < len(blocks); i++ {
		if blocks[i].MajorBlockIndex != blocks[i-1].MajorBlockIndex+1 {
			return fmt.Errorf("non-sequential block indices: %d followed by %d",
				blocks[i-1].MajorBlockIndex, blocks[i].MajorBlockIndex)
		}

		// Calculate next expected time using cron schedule
		expectedTime = schedule.Next(expectedTime)
		actualTimePtr := blocks[i].MajorBlockTime
		if actualTimePtr == nil {
			return fmt.Errorf("block %d has nil MajorBlockTime", blocks[i].MajorBlockIndex)
		}
		actualTime := *actualTimePtr
		if actualTime.Before(expectedTime.Add(-tolerance)) || actualTime.After(expectedTime.Add(tolerance)) {
			return fmt.Errorf("block %d timestamp %v does not match expected schedule %v (±%v)",
				blocks[i].MajorBlockIndex, actualTime, expectedTime, tolerance)
		}

		fmt.Printf("Block %d follows block %d with correct sequence and timestamp\n",
			blocks[i].MajorBlockIndex, blocks[i-1].MajorBlockIndex)
	}

	fmt.Printf("Block sequence and timestamp validation passed for %d blocks\n", len(blocks))
	return nil
}

// QueryNetworkGlobals fetches the NetworkGlobals record from the given partition URL.
func QueryNetworkGlobals(ctx context.Context, cl client.Client, partitionUrl *accurl.URL) (*protocol.NetworkGlobals, error) {
	globalsUrl := partitionUrl.JoinPath(protocol.Globals)

	record, err := cl.Query(ctx, &client.GeneralQuery{
		UrlQuery: client.UrlQuery{Url: globalsUrl},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query network globals: %w", err)
	}

	accountRecord, ok := record.(*api.AccountRecord)
	if !ok {
		return nil, fmt.Errorf("unexpected record type: %T", record)
	}

	// Manually convert to *protocol.NetworkGlobals
	data, err := json.Marshal(accountRecord.Account)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal account: %w", err)
	}

	var globals protocol.NetworkGlobals
	if err := json.Unmarshal(data, &globals); err != nil {
		return nil, fmt.Errorf("failed to unmarshal account as NetworkGlobals: %w", err)
	}

	return &globals, nil
}

// TrackAuthorityChanges builds up a timeline of authority keybook/keypage changes
// across the provided major blocks. This is necessary for dynamic signature verification
// as authorities may change over time.
//
// TODO: Implement authority tracking logic to handle keybook/keypage updates.
func TrackAuthorityChanges(ctx context.Context, cl client.Client, txID []byte) (*protocol.KeyBook, error) {
	// Parse transaction IDs
	txUrl, err := accurl.Parse(fmt.Sprintf("acc://%x", txID))
	if err != nil {
		return nil, fmt.Errorf("failed to parse transaction URL: %v", err)
	}

	txIdUrl, err := txUrl.AsTxID()
	if err != nil {
		return nil, fmt.Errorf("failed to convert URL to TxID: %v", err)
	}

	// Query the authority change transaction
	resp, err := cl.QueryTx(ctx, &client.TxnQuery{TxIdUrl: txIdUrl})
	if err != nil {
		return nil, fmt.Errorf("failed to query transaction: %v", err)
	}

	// For demonstration purposes, we'll just print the transaction type
	fmt.Printf("Authority Change Transaction:\n")
	if resp.Transaction != nil && resp.Transaction.Body != nil {
		fmt.Printf("  Type: %s\n", resp.Transaction.Body.Type())
	} else {
		fmt.Printf("  Type: <nil>\n")
	}
	fmt.Printf("  Signatures: %d\n", len(resp.Signatures))

	// In a real implementation, you would extract the new authority set
	// and return it for validating subsequent blocks

	return nil, nil
}

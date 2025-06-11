// This is a code snippet that can be added to the collectBPT function
// to count account types during snapshot collection

// Add these imports at the top of the file
import (
	"sort"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Add this code to the collectBPT function before the BPT iteration loop:
// Initialize counters for different account types
accountTypeCounters := make(map[protocol.AccountType]int)
unresolvedKeys := 0
totalEntries := 0

// Then modify the BPT iteration loop:
it := batch.BPT().Iterate(1000)
cnt := 0
for it.Next() {
	for _, entry := range it.Value() {
		key := batch.resolveAccountKey(entry.Key)
		err = wr.WriteValue(&snapshot.RecordEntry{
			Key:   key,
			Value: entry.Value[:],
		})
		if err != nil {
			fmt.Printf("[ERROR] Failed to write BPT entry (count=%d)\n", cnt)
			return errors.UnknownError.Wrap(err)
		}
		cnt++
		totalEntries++

		// Count account types
		if kh, ok := key.Get(0).(record.KeyHash); ok {
			// This is an unresolved key
			unresolvedKeys++
		} else if key.Get(0) == "Account" {
			// This is an account key
			if urlStr, ok := key.Get(1).(*url.URL); ok {
				// Try to get the account
				account, err := batch.Account(urlStr).Main().Get()
				if err == nil {
					// Count by account type
					accountTypeCounters[account.Type()]++
				}
			}
		}

		//AI: Log progress every 100000 entries to monitor BPT collection.
		if cnt%100000 == 0 {
			fmt.Printf("[INFO] Collecting BPT (progress) count=%s\n", humanize.Comma(int64(cnt)))
		}
	}
}

// Add this code after the BPT iteration loop to print the results:
fmt.Printf("[INFO] BPT Account Type Distribution:\n")
// Sort account types for consistent output
types := make([]protocol.AccountType, 0, len(accountTypeCounters))
for t := range accountTypeCounters {
	types = append(types, t)
}
sort.Slice(types, func(i, j int) bool {
	return types[i] < types[j]
})

// Print counts for each account type
for _, t := range types {
	count := accountTypeCounters[t]
	percent := float64(count) / float64(totalEntries) * 100
	fmt.Printf("[INFO] %-20s: %10s (%6.2f%%)\n", t.String(), humanize.Comma(int64(count)), percent)
}

// Print unresolved count
if unresolvedKeys > 0 {
	percent := float64(unresolvedKeys) / float64(totalEntries) * 100
	fmt.Printf("[INFO] %-20s: %10s (%6.2f%%)\n", "Unresolved Keys", humanize.Comma(int64(unresolvedKeys)), percent)
}

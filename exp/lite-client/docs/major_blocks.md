# Major Block Querying: Technical Progress & Open Questions

## Purpose of This Document
This is a technical progress report and feedback request regarding the lite client’s logic for querying and processing major blocks in Accumulate. It is intended for review by Paul or other protocol maintainers.

---

## 1. What Has Been Implemented

- **Sequential Major Block Querying:**
  - The client queries major blocks from index 0 to latest using a paginated approach.
  - Uses `QueryMajorBlocks` (see `blocks/block_major.go`) to fetch a range of blocks at a time.
- **Authority Set Extraction:**
  - For each major block, the authority set (validator keys, threshold, index) is extracted for signature validation and governance tracking.
- **Authority Tracker Construction:**
  - Builds an `AuthorityTracker` mapping block indices to authority sets, enabling efficient validation of signatures and authority transitions.
- **Debug Logging:**
  - Added debug output to log partition URL, query parameters, and raw API responses to diagnose empty or unexpected results.

---

## 2. Current Querying Approach (Code Logic)

- The client constructs a query with a start index and count (range) and sends it to a specific partition URL.
- The partition URL is currently hardcoded (e.g., `acc://bvn0.acme`), but can be parameterized.
- The query is executed via `api.Querier2.QueryMajorBlocks`, returning a `RecordRange[*MajorBlockRecord]`.
- Results are processed into maps for downstream use.

**Key code excerpt:**
```go
partitionUrl, err := parseUrl("acc://bvn0.acme") // TODO: parameterize this
query := createQueryMajorBlock(startIndex, &count, partitionUrl)
resp, err := executeQueryMajorBlock(ctx, partitionUrl, querier2, query)
// Debug: print the raw response
fmt.Printf("Raw response from executeQueryMajorBlock: %+v\n", resp)
```

---

## 3. Pitfalls & Uncertainties

- **Partition URL Selection:**
  - Using the wrong partition (e.g., a BVN without major blocks) leads to empty results.
  - Directory Network (`acc://dn`) is most likely to contain major blocks, but this is not guaranteed for all networks/configs.
- **Data Availability:**
  - Querying a range with no blocks (e.g., beyond chain head) yields empty responses.
- **Hardcoded Parameters:**
  - Hardcoding partition URLs or query ranges is brittle. These should be configurable.
- **Testnet/Node Sync:**
  - Some testnets or local nodes may not have major blocks or may not be fully synced, resulting in empty data.

**Example debug output for empty result:**
```
Raw response from executeQueryMajorBlock: &{Records:[]}
Raw response records: count=0, records=[]
Partition URL used: acc://bvn0.acme
```

---

## 4. Open Questions

- Is the current approach for selecting the partition URL sound? Should we always default to Directory Network (`acc://dn`), or is there a better practice?
- Are there edge cases or partition-specific behaviors I should be aware of when querying major blocks?
- Is there a canonical way to discover which partitions have major blocks on a given network?
- Any feedback on the code structure, parameterization, or documentation?

---

## 5. Next Steps (Pending Feedback)
- Parameterize partition URL selection.
- Add logic to auto-discover available partitions if possible.
- Improve error handling and user-facing diagnostics.
- Update documentation and code based on reviewer (Paul’s) feedback.

---

**Please review and let me know if the approach and rationale make sense, or if there are protocol nuances I should address.**

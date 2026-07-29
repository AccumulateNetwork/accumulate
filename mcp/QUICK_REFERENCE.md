# Accumulate SDK Quick Reference

## Directory Structure
- **API Interface**: `/pkg/api/v3/` - Main API definitions
- **Queries**: `/pkg/api/v3/queries.yml` - Query type definitions
- **Records**: `/pkg/api/v3/records.yml` - Response/record type definitions
- **Transactions**: `/protocol/user_transactions.yml`, `/protocol/synthetic_transactions.yml`
- **Accounts**: `/protocol/accounts.yml`
- **JSON-RPC**: `/pkg/api/v3/jsonrpc/services.go` (11 methods defined)
- **REST**: `/pkg/api/v3/rest/` (30+ endpoints)

## API v3 Overview
- **Version**: v1.4.2
- **Transport**: JSON-RPC 2.0, REST, P2P, WebSocket
- **Design**: Service-based with single method per service
- **Services**: 11 services with ~20 methods total

## 11 Services & Key Methods

| Service | Primary Method | Purpose |
|---------|---|---|
| NodeService | NodeInfo, FindService | Network node info & service discovery |
| ConsensusService | ConsensusStatus | Consensus node status |
| NetworkService | NetworkStatus | Network globals & routing |
| SnapshotService | ListSnapshots | System snapshots |
| MetricsService | Metrics | Network TPS metrics |
| Querier | Query | Main read interface (11 query types) |
| EventService | Subscribe | Event streaming (3 event types) |
| Submitter | Submit | Transaction submission |
| Validator | Validate | Pre-validate transactions |
| Faucet | Faucet | Request ACME tokens |
| Sequencer | Sequence | Private sequencing API |

## Query Types (11 total)

**Standard Queries:**
1. DefaultQuery (0x00) - Basic account/tx query
2. ChainQuery (0x01) - Chain entries with name, index, or entry search
3. DataQuery (0x02) - Data account entries
4. DirectoryQuery (0x03) - Identity directory
5. PendingQuery (0x04) - Pending transactions
6. BlockQuery (0x05) - Major/minor blocks

**Search Queries:**
7. AnchorSearchQuery (0x10) - Search anchors
8. PublicKeySearchQuery (0x11) - Search public keys
9. PublicKeyHashSearchQuery (0x12) - Search key hashes
10. DelegateSearchQuery (0x13) - Search delegates
11. MessageHashSearchQuery (0x14) - Search by message hash

## Record Types (13 total)

**Data Records:**
- AccountRecord (0x01) - Account with directory/pending
- ChainRecord (0x02) - Chain metadata
- ChainEntryRecord (0x03) - Generic chain entry
- KeyRecord (0x04) - Key information

**Message Records:**
- MessageRecord (0x10) - Transactions/signatures/messages
- SignatureSetRecord (0x11) - Signature collection

**Block Records:**
- MinorBlockRecord (0x20)
- MajorBlockRecord (0x21)

**Utility Records:**
- RecordRange (0x80) - Paginated results
- UrlRecord (0x81) - URL wrapper
- TxIDRecord (0x82) - Transaction ID wrapper
- IndexEntryRecord (0x83) - Index entry wrapper
- ErrorRecord (0x8F) - Error response

## Transaction Types

**User Transactions (23):**
CreateIdentity, CreateTokenAccount, SendTokens, CreateDataAccount, WriteData, WriteDataTo, AcmeFaucet, CreateToken, IssueTokens, BurnTokens, CreateLiteTokenAccount, CreateKeyPage, CreateKeyBook, AddCredits, BurnCredits, TransferCredits, UpdateKeyPage, LockAccount, UpdateAccountAuth, UpdateKey, NetworkMaintenance, ActivateProtocolVersion, RemoteTransaction

**Synthetic Transactions (6):**
SyntheticCreateIdentity, SyntheticWriteData, SyntheticDepositTokens, SyntheticDepositCredits, SyntheticBurnTokens, SyntheticForwardTransaction

## Account Types (11 total)
**Standard:** ADI, TokenAccount, DataAccount, TokenIssuer, KeyBook, KeyPage
**Lite:** LiteIdentity, LiteTokenAccount, LiteDataAccount
**System:** UnknownAccount, UnknownSigner

## Event Types (3)
1. ErrorEvent - Error notification
2. BlockEvent - Block commitment with partition, index, time, major, entries
3. GlobalsEvent - Network globals change

## Signature Types (8)
LegacyED25519, ED25519, RCD1, BTC, BTCLegacy, ETH, Delegated, Authority

## REST API Endpoints (~30)

**Query Endpoints:**
- GET /query/{id}
- GET /query/{id}/chain | /chain/{name} | /chain/{name}/index/{index} | /chain/{name}/entry/{hash}
- GET /query/{id}/data | /data/index/{index} | /data/entry/{hash}
- GET /query/{id}/directory | /pending
- GET /block/minor | /block/major | /block/minor/{index} | /block/major/{index}
- GET /search/{id}/anchor/{value} | /publicKey/{value} | /delegate/{value}

**Service Endpoints:**
- GET /node/info | /node/services
- GET /consensus/status
- GET /network/status
- GET /metrics
- POST /submit | /validate | /faucet

**JSON-RPC:**
- POST /v3

## JSON-RPC Methods (11)
node-info, find-service, consensus-status, network-status, list-snapshots, metrics, query, submit, validate, faucet, private-sequence

## Common Options

**RangeOptions** (pagination):
- Start: Starting index
- Count: Number of results
- Expand: Resolve nested values
- FromEnd: Query from end

**Other Options:**
- ReceiptOptions: ForAny, ForHeight
- SubmitOptions: Verify, Wait
- ValidateOptions: Full
- FaucetOptions: Token
- SubscribeOptions: Partition, Account

## Status Codes (~20)
OK, Delivered, Pending, Remote, WrongPartition, BadRequest, Unauthenticated, InsufficientCredits, Unauthorized, NotFound, NotAllowed, Rejected, Expired, Conflict, BadSignerVersion, BadTimestamp, BadUrlLength, IncompleteChain, InsufficientBalance, InternalError, UnknownError, EncodingError, FatalError, NotReady, WrongType, NoPeer, PeerMisbehaved, InvalidRecord

## Key Architectural Points

1. **Single Method Services** - Each service has 1 method for flexibility
2. **Union Types** - Query, Record, Event, Account are all unions
3. **Generic Records** - ChainEntryRecord[T], MessageRecord[T], RecordRange[T]
4. **Pagination** - RangeOptions for streaming results
5. **Multi-Sig Support** - SignatureSetRecord handles multiple signers
6. **Chain Proofs** - Optional receipt inclusion for chain proofs
7. **Network Awareness** - Partition, node discovery, routing tables

## Implementation Priority for MCP
1. **Core Queries** (DefaultQuery, ChainQuery, DataQuery) - 80% of use cases
2. **Transaction Submission** (Submit, Validate, Faucet)
3. **Network Status** (NodeInfo, NetworkStatus, ConsensusStatus, Metrics)
4. **Advanced Queries** (Search, Block queries)
5. **Event Subscriptions** (complex streaming)
6. **Snapshots** (administrative)

## Key URLs & Patterns
- Format: `acc://[hash@]domain[/path]`
- Example: `acc://alice.acme` (identity)
- Example: `acc://alice.acme/tokens` (sub-account)
- Example: `acc://hash@account` (transaction/message)

## Go Module Location
`gitlab.com/accumulatenetwork/accumulate@v1.4.2`

## Important Files for Reference
- /pkg/api/v3/api.go - Service interfaces
- /pkg/api/v3/querier.go - Query helpers
- /pkg/api/v3/enums.yml - All enum definitions
- /pkg/api/v3/openapi.yml - Complete OpenAPI spec
- /protocol/general.yml - Common protocol types


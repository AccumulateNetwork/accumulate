# sidecar-extract

Builds the **sidecar**: the history mainnet lost in the 13 July 2025 reorg (#4270), recovered from pre-reorg databases into a separate LevelDB store that a node reads when its live database misses (#4273). This text is the tool's spec, carried from `docs/spec/database.md` on dagbft-integration (main has no `docs/spec/`).

**The built sidecar** (2026-09-24, at 148c148e2, against server5's follower copy): `sidecar.db` is 182 GB and holds 718,415,890 records (200 GB of values) from six read-only archives (Directory B/A, Apollo B/A, Yutu A, Chandrayaan A). Beside it, `conflicts.db` (1.3 GB) holds 5,529,964 keys that two partitions hold with different values. 9,585 Directory keys (February 2025, Directory blocks 32,867,745–33,044,270) were in no copy; 2,499 of them (chain elements and mark points) were rebuilt from mark points and blocks and proven, and 7,086 remain lost, listed in `lost-9585.tsv` beside it; see #4273.

## Recovered history

The 13 July 2025 reorg rebuilt mainnet from a filtered snapshot, and the filter
dropped history (#4270). Pre-reorg databases survive. What they hold that the
current database does not is carried in a **sidecar**: a separate store, read
when the live database misses (#4273).

**The sources.** Two independent pre-reorg copies exist, both Badger v1:

| Copy | Partitions | Last signed |
|---|---|---|
| A — the four validators' databases | Yutu, Apollo, Chandrayaan, Directory | 3 June 2025 |
| B — the Apollo host's node | Apollo, Directory | Directory 22 June 2025; block store to 13 July |

Copy B is newer for the partitions it has; for Yutu and Chandrayaan, copy A is
all there is. Every copy is used as the least-written version available and is
never opened writable in place: a writable Badger open compacts and collects
garbage, and garbage collection on an index with stale pointers discards the
very entries recovery needs.

**What the sidecar holds**: every pre-reorg record whose key no current
partition's database holds. The difference is taken on raw store keys, so
nothing is decoded and no record kind is chosen or left out; a key the live
database has is the live database's, whatever the archive held for it.
Because keys are `Key.Hash()`, one sidecar serves every current partition.

**One value per partition, then one value per key.** For each partition, the
newest copy with a readable value speaks. A key two partitions hold with
different values is not in the sidecar: each partition's value is kept in a
separate conflicts store under the key followed by the archive's index, for a
decision per record kind; the sidecar never picks one.

**Every value is verified.** Badger v1 does not check what it reads from its
value log, and in these archives the index points at log offsets that were
later overwritten — at garbage, or at another record's intact entry, which
Badger returns without complaint. A value is taken only from an intact log
entry (checksum) for exactly its key and version, following Badger's move entry
where garbage collection relocated it. A value that fails is looked for by key
and version in a scan of its copy's whole log; what no copy can produce is
listed as lost, never guessed.

**Built offline, read-only**: `tools/cmd/sidecar-extract` merges the copies in
key order against the current key set and writes a LevelDB sidecar with the
node's bloom filter. It writes nothing to any source. The current databases are
copied from a stopped follower.

**Reading it**: the `overlay` backend composes the live store over the
sidecar, reads falling through on a miss. A hit is written back to the live
store, so the sidecar can be retired once what is read has been repaired.
Which records the read path may fall through for — a key the node deliberately
pruned must stay absent — is decided on the read side, not by what the sidecar
holds. Records that feed `hashChains` (anchor chains, the BPT) are never served
from it: they are consensus state, restored only by #4020's mechanism.


**Serving it from another node** (#4561): the sidecar is deployed to one node, and other nodes read it over the network instead of each carrying it. That adds a hop to a read that misses, and saves every other node the sidecar's disk.

- The serving node runs a `SidecarService` (`Path`: the sidecar database, `Listen`: its addresses). It answers `GET /key/<hex>`, where the key is the record's 32-byte storage key, with the raw value, or 404 when it does not hold it. It only reads.
- A reading node sets `Sidecar: <url>` on its `Querier` service. A read the API makes that misses the live database is read from the sidecar (`pkg/database/keyvalue/sidecar`).
- **Only the API reads it.** The executor and every other service read the live database alone, so what a node agrees on cannot depend on whether it has a sidecar.
- **What falls through, and what is kept, is the policy** (#4560, `sidecar.Policy`). The records that feed `hashChains` for the partition the node serves — its BPT, its ledger, its anchor pool — never fall through. Everything else may, including the old partitions' ledgers, which the executor never reads and the receipt builder needs. Of what falls through, only records the live node will never write are written back into the live database, in the background, so the hop is paid once: messages and transactions, keyed by hash, and the elements, mark points, element indexes and intermediates of chains, which are append-only and were kept whole through the reorg. A data index entry is not kept (the reorg restarted those counters, so the live node writes the same keys again); nor is a chain head, a pending set, or an account's state.
- **It fails soft.** A sidecar that is unreachable, slow (2 s) or broken leaves the read with the live database's not-found, as for a node without one, and the failure is logged.

**The era** (#4567): an index chain entry records the root chain height its anchor landed at, but not whose root chain. The reorg moved every account to Cyclops and restarted the Directory's root chain under the same keys, so every index entry written before it names a height in a root chain the live node does not have. Which one is routed, not recorded: the routing table in force before the reorg names the partition the account lived on, and that partition's root chain, read as it was then, is the one the entry anchors into.

- The serving node's `SidecarService` takes `Era`: the directory of the overwritten store (`-keep-overwritten`: the archives' values for the keys the live database holds with another value, under key‖archive index). It then answers `GET /era/<hex>` with the value the key had before the reorg — the overwritten store's, the first archive in index order that holds it, else the sidecar's.
- The reading node's querier reads the era as the sidecar's era in front of its live store (`indexing.Era`), reads the pre-reorg routing table from it once (`dn.acme/routing`'s main state), and builds a chain receipt (`indexing.ChainReceiptFor`) by testing which root chain the index entry proves against: the live partition's first, then the routed partition's pre-reorg one through the era. An index entry proves against a root chain when that chain's element at the entry's anchor height is the account chain's anchor at the entry's source. The receipt's block is in the numbering of the root chain it terminates at, and `Receipt.Partition` names that partition when it is not the serving one.
- The restore's own block-1 entries on a few Directory accounts anchor a state their chain never had, so they prove against neither root chain. The genesis element such an entry covers is proved through the next index entry of the live era, which covers it too.
- The era is never written back.

**Restarted data indexes** (#4273, Paul's rule: map the counts to what they would be had the restore not restarted them). The genesis restore started every data index it rebuilt at zero, so a live index counts only the entries written since, and its entry *i* is the *i*'th since the reorg while the pre-reorg index's entry *i* is the *i*'th before it. The querier serves the index as it would have been (`dataView`): the pre-reorg entries first, then the live ones numbered on from where the pre-reorg index ended, with the total as the sum. A restarted index is told from a whole one by one read — a restarted index has an entry in the era at the live count, a whole one cannot. The pre-reorg count is the era's count where an overwritten store holds it, else the end of the era's run of entries, found by doubling and bisection. It is remembered per account per process. The entries below the live count are the pre-reorg index's only where an overwritten store holds them: `-keep-overwritten-data` builds one for every data account the live copy holds (its count and every entry below the live count, from each archive, when it differs), and the serving node's `Era` lists its directory after the Directory's. Entries at and beyond the live count the sidecar holds.

**Mounting it locally** (#4468): a LevelDB node's storage takes `Sidecar: <path>`; a read that misses the primary falls through to the sidecar and is written back to the primary in the background (`pkg/database/keyvalue/overlay`, repair-on-hit). This puts the sidecar under every reader, the executor included, and no rule yet keeps a deliberately pruned key, or consensus state, from falling through (#4560). Serving it from another node is the deployed form.

## Checking what it answers

Two modes read the sidecar beside a copy of the live databases, every store read-only, and say what the queries that motivated it (#4268, #4269) get:

- `-check <sidecar> -current <live>... [-accounts <list>] [-data-only] [<account>...]` makes the calls the bridge makes — query-tx-history, query-data for the newest entry, query-data-set over every entry — through the API's own `Querier`, from the live databases alone and then with the sidecar behind them. Per account and layering it reports the main chain's entries and messages read and their transaction types, the data index's entries read, whether the newest-entry query answers, and a data index the live store **restarted** after the reorg: a count of its own that hides the sidecar's entries at and beyond it. `-data-only` skips accounts that are not data accounts; the run ends with one summary line per layering.
- `-where -sidecar <sidecar> -conflicts <conflicts> -current <live>... <account>...` says which store holds each of an account's records — main state, main chain head, each main-chain entry's message (under `Message` or the older `Transaction` key), and the data index's count, entries and entry-to-transaction map: `live`, `sidecar`, `conflicts[...]` with the archives that hold it grouped by value, or `none`. With `-partition <P>[,<P>...]` it also tests the account's main-index chain against each named partition's root chain, read through live and the sidecar: an index entry proves when the root chain entry at its `Anchor` is the main chain's anchor at its `Source`. An index entry records the root chain height its anchor landed at but not whose root chain, so an entry written on a partition the account no longer lives on proves only against that partition's root chain (#4273). For each partition it also says how far the directory's `anchor(P)-root` chain reaches, which is the second hop of such a receipt.
- `-route -route-archive <name>=<path> -partition <live> -sidecar <sidecar> -current <live>... <account>...` routes each account with the routing table in force before the reorg and tests its main-index chain against that partition's root chain first and the live partition's second. The pre-reorg table is the last entry of `dn.acme/routing`, whose live account was restarted; a chain under 256 entries keeps its hashes in its head record, so the sidecar does not hold the old entries and the table is read from a directory archive's copy of the account's main state. Across the 157 ADI data accounts on the Cyclops follower copy every one of 134,125 index entries proved: 133,591 on the routed partition's root chain and 534 on Cyclops. Accounts routed to the Directory prove only their post-reorg entries, unless the pre-reorg Directory root chain is reachable; see `-keep-overwritten` (#4567).
- `-keep-overwritten -out <dir> -current <live>... <name>=<archive>...` builds `overwritten.db` beside the sidecar: for every key the live databases hold, each archive's value for it when it differs, under the key followed by the archive's index (`overwritten.json` lists the archives in order). The extractor leaves such keys to the live database, so a chain the live database restarted under the same keys — the Directory's root chain, its anchor chains and `dn.acme/routing` at the reorg — is in no store it builds; this one holds the archive's side. Read with `-overwritten <path>`, `-route` sees the databases as they were before the reorg: the overwritten store first, then live, then the sidecar.
- **The transition.** Index entries are in order and block numbers only rise within an era, so the first entry whose block number is below the one before it is the restore's (block 1). Entries before it anchor into the root chain of the partition the account routed to before the reorg, read in that era; it and everything after anchor into the live partition's. A chain with no such entry is all one era. On the Cyclops follower copy, 157 ADI data accounts: 55 have a transition and 102 are all since the reorg; 133,591 of 133,591 entries before the transition prove on the routed partition and 534 of 534 since prove on Cyclops. On the Directory copy with the overwritten store, staking.acme/registered proves 150 of 150 before and staking.acme/requests 163 of 163, and the directory's own `anchor(Directory)-root` chain of that era holds the block's root anchor.


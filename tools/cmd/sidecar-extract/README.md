# sidecar-extract

Builds the **sidecar**: the history mainnet lost in the 13 July 2025 reorg (#4270), recovered from pre-reorg databases into a separate LevelDB store that a node reads when its live database misses (#4273). This text is the tool's spec, carried from `docs/spec/database.md` on dagbft-integration (main has no `docs/spec/`).

**The built sidecar** (2026-09-24, at 148c148e2, against server5's follower copy): `sidecar.db` is 182 GB and holds 718,415,890 records (200 GB of values) from six read-only archives (Directory B/A, Apollo B/A, Yutu A, Chandrayaan A). Beside it, `conflicts.db` (1.3 GB) holds 5,529,964 keys that two partitions hold with different values. Exactly 9,585 Directory keys (February 2025, Directory blocks 32,867,745–33,044,270) are in no copy; see #4273.

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


**Mounting it** (#4468): a LevelDB node's storage takes `Sidecar: <path>`; a read that misses the primary falls through to the sidecar and is written back to the primary in the background (`pkg/database/keyvalue/overlay`, repair-on-hit). Still open: no rule yet keeps a deliberately pruned key, or consensus state, from falling through — the overlay falls through for every miss.

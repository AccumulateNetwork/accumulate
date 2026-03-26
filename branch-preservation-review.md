# Branch Preservation Review

Generated: 2026-03-25 18:00:01

## Executive Summary

- **Total branches**: 393
- **Essential branches (KEEP)**: 10
- **Active development (0-30 days, KEEP)**: 31
- **Release branches (REVIEW)**: 35
- **Recent merged (30-90 days, CAN DELETE)**: 0
- **Stale merged (90+ days, DELETE)**: 0
- **Stale unmerged (90-365 days, REVIEW)**: 45
- **Very old unmerged (1+ years, LIKELY DELETE)**: 272

### Age Distribution

- Last week: 29
- Last month: 4
- 1-3 months: 4
- 3-6 months: 19
- 6-12 months: 26
- 1-2 years: 22
- 2+ years: 289

### Merge Status

- Merged to dagbft-integration: 40
- Merged to main: 9
- Unmerged: 353

---

## 1. Essential Branches (KEEP)

These are protected branches that must be preserved.

- `origin/aip-53-lxr-mining-base` - Last updated: 2025-06-13 (285 days ago)
- `origin/dagbft-integration` - Last updated: 2026-03-25 (0 days ago)
- `origin/develop` - Last updated: 2022-10-18 (1254 days ago)
- `origin/hotfix-main` - Last updated: 2023-04-11 (1079 days ago)
- `origin/main` - Last updated: 2025-06-13 (285 days ago)
- `origin/master` - Last updated: 2022-10-12 (1260 days ago)
- `origin/not-develop` - Last updated: 2021-09-30 (1637 days ago)
- `origin/old-master` - Last updated: 2021-09-07 (1660 days ago)
- `origin/stepapp1-develop-patch-61548` - Last updated: 2022-01-05 (1540 days ago)
- `origin/test-mainnet` - Last updated: 2022-12-22 (1189 days ago)

---

## 2. Active Development (KEEP)

Branches with activity in the last 30 days. These are actively being worked on.

**Count**: 31

- `origin/feature/issue-cleanup-64-issues` - 0 days ago [MERGED]
  - Close 65 obsolete and completed issues
- `origin/issue-3742-bullshark-ordering-fix` - 0 days ago [MERGED]
  - Fix missing crypto/rand import in key_comet.go
- `origin/issue/dagbft-3815` - 0 days ago [MERGED]
  - Issue #3815: Implement BPT sync recovery
- `origin/issue-3871-key-rotation` - 0 days ago [MERGED]
  - Issue #3871: Add validator integration and API endpoints for key rotation
- `origin/issue-3875-per-peer-vote-rate-limiting` - 0 days ago [MERGED]
  - Apply eviction optimization to worker.go for issue #3873
- `origin/feature/issue-3874` - 0 days ago [MERGED]
  - Issue #3870: Reorder vote verification checks to prevent CPU exhaustion
- `origin/issue-3872-timestamp-replay-protection` - 0 days ago [MERGED]
  - Issue #3872: Document timestamp replay protection requirements
- `origin/feature/issue-3863` - 2 days ago [MERGED]
  - Fix e2e2/generated working directory error (#3863)
- `origin/feature/issue-3857` - 2 days ago [MERGED]
  - Fix database concurrency bug by returning conflict errors (#3857)
- `origin/feature/issue-3858` - 2 days ago [MERGED]
  - Fix duplicate key validation for empty key hashes in createKeyPage (#3858)
- `origin/feature/issue-3854` - 3 days ago [MERGED]
  - Add integration tests and fix load generator bug (#3854)
- `origin/feature/issue-3855` - 3 days ago [MERGED]
  - Add integration testing and debugging for monitoring & analysis (#3855)
- `origin/feature/issue-3850` - 3 days ago [MERGED]
  - Add comprehensive tests for load generator transaction logic (#3850)
- `origin/feature/issue-3853` - 3 days ago [MERGED]
  - Add comprehensive tests for test reporting tool (#3853)
- `origin/feature/issue-3851` - 3 days ago [MERGED]
  - Add comprehensive tests for monitoring dashboard and metrics (#3851)
- `origin/feature/issue-3852` - 3 days ago [MERGED]
  - Add documentation for performance monitoring tool (#3852)
- `origin/feature/issue-3849` - 3 days ago [MERGED]
  - Add comprehensive tests for init-test-data tool (#3849)
- `origin/feature/issue-3844` - 3 days ago [MERGED]
  - Add performance monitoring and tuning system (#3844)
- `origin/feature/issue-3845` - 3 days ago [MERGED]
  - Add real-time monitoring dashboard for load testing (#3845)
- `origin/feature/issue-3843` - 3 days ago [MERGED]
  - Fix URL parsing in load generator (#3843)
- `origin/feature/issue-3847` - 3 days ago [MERGED]
  - Add testreport binary to gitignore (#3847)
- `origin/feature/issue-3846` - 3 days ago [MERGED]
  - Add test results database for DAG-BFT testing framework (#3846)
- `origin/issue/dagbft-3821` - 6 days ago [MERGED]
  - Review rangevarref lint fix (#3821)
- `origin/issue/dagbft-3819` - 6 days ago [MERGED]
  - Add review report for copylocks fix (#3819)
- `origin/issue/dagbft-3818` - 6 days ago [MERGED]
  - Add review report for SA1026 fix (#3818)
- `origin/issue/dagbft-3820` - 6 days ago [MERGED]
  - Add review report for exp/light logger migration (#3820)
- `origin/issue/dagbft-3822` - 6 days ago [MERGED]
  - Add review report for ineffassign lint fixes (#3822)
- `origin/fix/lint-cleanup` - 6 days ago [MERGED]
  - Fix staticcheck lint warnings in consensus packages
- `origin/3718-cometbft-analysis` - 9 days ago [MERGED]
  - Issue #3756: Add DAG-BFT configuration package
- `origin/issue-3742-fix-leader-chain-traversal` - 9 days ago [MERGED]
  - Issue #3742: Fix leader chain traversal to skip missing leaders
- `origin/3714-sdk-signature-docs` - 25 days ago
  - Add SDK signature documentation and examples [#3714]

---

## 3. Release Branches (REVIEW)

Release and version branches. Old releases can likely be deleted.

**Count**: 35

- `origin/3702-release-1.4.4-beta.3` - 14 days ago
- `origin/3701-release-1.4.4-update` - 93 days ago
- `origin/release-1.4.4-update` - 95 days ago
- `origin/release-1.4` - 411 days ago
- `origin/release-1.3` - 751 days ago [MERGED]
- `origin/release-1.2` - 778 days ago [MERGED]
- `origin/release-1.1` - 984 days ago
- `origin/release-1.0` - 1076 days ago [MERGED]
- `origin/hotfix-1.0.4` - 1086 days ago
- `origin/regenesis-1.0` - 1161 days ago
- `origin/dev-1.0.2` - 1175 days ago
- `origin/cli-v1.0.0-rc2` - 1303 days ago
- `origin/cli-v1.0.0-rc1.2-debug` - 1329 days ago
- `origin/cli-v1.0.0-rc1.2` - 1329 days ago
- `origin/cli-v1.0.0-rc1.1` - 1336 days ago
- `origin/cli-v1.0.0-rc1` - 1344 days ago
- `origin/cli-0.9.1-beta` - 1362 days ago
- `origin/cli-0.9.0-beta` - 1373 days ago
- `origin/cli-0.8.2-beta` - 1380 days ago
- `origin/cli-0.8.1-beta` - 1386 days ago
- `origin/cli-0.8.0-beta` - 1387 days ago
- `origin/release-v0.8.0-beta` - 1387 days ago
- `origin/qa-0.7.0-beta` - 1400 days ago
- `origin/cli-0.7.0-beta` - 1401 days ago
- `origin/release-v0.6.1` - 1423 days ago [MERGED]
- `origin/release-v0.6.0-rc2` - 1430 days ago [MERGED]
- `origin/release-v0.6.0-rc1` - 1434 days ago
- `origin/release-v0.6.0-rc0` - 1434 days ago
- `origin/release-v0.5.1` - 1446 days ago
- `origin/cli-v0.4` - 1455 days ago
- `origin/release-v0.4` - 1506 days ago
- `origin/release-v0.3` - 1541 days ago [MERGED]
- `origin/merge-release-v0.2` - 1598 days ago
- `origin/release-v0.2` - 1598 days ago
- `origin/tendermint-0.35.0-rc1` - 1645 days ago

---

## 4. Recent Merged Branches (CAN DELETE)

Branches merged to dagbft-integration within the last 90 days. Safe to delete if work is complete.

**Count**: 0


---

## 5. Stale Merged Branches (DELETE)

Branches merged to dagbft-integration over 90 days ago. Safe to delete.

**Count**: 0


---

## 6. Stale Unmerged Branches (REVIEW)

Branches not merged, 90-365 days old. Likely obsolete unless containing important work.

**Count**: 45

- `origin/healing_update` - 362 days ago
  - update
- `origin/3647-healing-update` - 337 days ago
  - reset network.go
- `origin/healing-hack` - 296 days ago
  - update limit anchor checking constant
- `origin` - 285 days ago
  - Update of the lite client design and supporting API documentation
- `origin/fix-init-dual-part-2` - 254 days ago
  - Fix partition type selection to work with new nodes
- `origin/ethan` - 252 days ago
  - added debug command for cyclops
- `origin/3652-create-a-genesis-block-2` - 246 days ago
  - Documentation consolidation and cleanup before removing large files
- `origin/3651-phase-1-account-proof-for-lite-client` - 238 days ago
  - checkpoint
- `origin/3653-add-a-crosschainconductor-process-for-coordinating-partitions_pipeline` - 227 days ago
  - fix: resolve GitLab CI pipeline failures - attempt #1
- `origin/3656-implement-unified-go-client-package-for-accumulate-apis` - 225 days ago
  - fix: resolve mainnet P2P connectivity and DN height issues
- `origin/3659-crosschain-conductor-implementation` - 221 days ago
  - feat: add collection proofs sub-issue for CrossChain Conductor
- `origin/3661-sdk-connection-management` - 219 days ago
  - feat: start P2P reliability improvements and SDK connection management
- `origin/3660-activate-collection-proofs` - 219 days ago
  - fix: refactor load testing infrastructure and address nil pointer issues
- `origin/3662-ccc-docs-reorganization` - 219 days ago
  - docs: import CrossChain Conductor documentation from 3660
- `origin/3658-v1-5-0` - 211 days ago
  - chore: add scripts/devnet/ to .gitignore
- `origin/3658-cryptographic-proof-api` - 211 days ago
  - feat(api): implement consensus proof support for trustless verification
- `origin/3664-api-support-for-cryptographic-proof-system-in-lite-client` - 210 days ago
  - feat: Add devnet configuration scripts
- `origin/3680-lxr-mining-baseline-clean` - 207 days ago
  - Merge branch '3683-accumulated-run-devnet-bvns-flag-ignored-always-creates-3-bvns-regardless-of-configuration' into 3680-lxr-mining-baseline-clean
- `origin/3652-create-a-genesis-block` - 207 days ago
  - Merge branch '3683-accumulated-run-devnet-bvns-flag-ignored-always-creates-3-bvns-regardless-of-configuration' into 3652-create-a-genesis-block
- `origin/3663-bogus` - 207 days ago
  - docs(devnet): Add comprehensive technical analysis of Issue 3683 fix
- `origin/3683-accumulated-run-devnet-bvns-flag-ignored-always-creates-3-bvns-regardless-of-configuration` - 207 days ago
  - docs(devnet): Add comprehensive technical analysis of Issue 3683 fix
- `origin/healing-anchor-synth-repair` - 205 days ago
  - docs: add healing anchor/synth repair issue description
- `origin/3653-add-a-crosschainconductor-process-for-coordinating-partitions` - 205 days ago
  - chore: remove redundant load testing files
- `origin/3684-crosschain-healing` - 201 days ago
  - Add mandatory tracking_repo review and TDD simplification requirements
- `origin/3665-lxr-mining-clean` - 163 days ago
  - fix: format imports with gosimports
- `origin/3669-mining-account-types-clean` - 158 days ago
  - feat: Implement Mining Account Types for AIP-53 LXR Mining (Issue #3669)
- `origin/3668-mining-transaction-clean` - 158 days ago
  - fix: Address code review recommendations for Mining Transaction
- `origin/3666-keypage-mining-fields-clean` - 158 days ago
  - fix: Clean up whitespace and formatting issues
- `origin/3680-lxr-mining-docs` - 158 days ago
  - docs: add baseline documentation for LXR mining feature
- `origin/3669-mining-account-types` - 158 days ago
  - feat: Implement Mining Account Types for AIP-53 LXR Mining (Issue #3669)
- `origin/3666-keypage-mining-fields-rebased` - 158 days ago
  - cleanup: Remove transient merge request template files
- `origin/3666-keypage-mining-fields` - 158 days ago
  - feat: Complete AIP-53 miner-as-validator specification with KeyPage mining fields
- `origin/3676-mining-epoch-management` - 157 days ago
  - feat: Implement Mining Epoch Management System (AIP-53 #3676)
- `origin/3675-mining-validator-component` - 157 days ago
  - feat: Implement Mining Validator Component for AIP-53 LXR Mining (Issue #3675)
- `origin/fix/crypto-arm64-compatibility` - 156 days ago
  - feat: Replace btcec/v2 and ethereum crypto dependencies for ARM64 compatibility
- `origin/3691-mcp-server-for-accumulate` - 113 days ago
  - Add MCP snapshot creation tool and integration tests
- `origin/3695-eliminate-need-for-observer` - 95 days ago
  - Merge branch '3700-halt-at-major-block' into 3695-eliminate-need-for-observer
- `origin/3700-halt-at-major-block` - 95 days ago
  - Add admin API to halt node at next major block
- `origin/3690-replace-btcec-v2-dependencies-fix-arm64-compilation` - 93 days ago
  - Fix ARM64 crypto implementation with proper secp256k1 support
- `origin/3704-api-stale-block-heights` - 92 days ago
  - Add staleness detection fields to API for issue #3704
- `origin/3703-blocksync-sync-detection` - 92 days ago
  - Fix pre-existing test failures in snapshot and state tests
- `origin/3697-lite-account-delegation` - 84 days ago
  - style: Format imports with gosimports
- `origin/3705-add-transaction-whitelist-to-keypage` - 75 days ago
  - Revert "Update CLAUDE.md with comprehensive build requirements"
- `origin/3713-add-version-commands` - 73 days ago
  - Consolidate version formatting into single implementation [#3713]
- `origin/3709-ethereum-data-entry` - 73 days ago
  - Remove unused recoverPubkey function to fix lint error (#3709)

---

## 7. Very Old Unmerged Branches (LIKELY DELETE)

Branches over 1 year old and never merged. Likely obsolete.

**Count**: 272

- `origin/add-chain-validation-engine` - 1709 days ago (2021-07-20)
  - wip: updating comments
- `origin/add-validator-opcodes` - 1706 days ago (2021-07-23)
  - opcode experimenting
- `origin/expand-jsonrpc-api` - 1701 days ago (2021-07-28)
  - fix typo
- `origin/provide-helpers-for-internal-api` - 1701 days ago (2021-07-28)
  - added some json rpc helpers
- `origin/AC-206_Refactor_Network` - 1656 days ago (2021-09-11)
  - merge
- `origin/AC-263-separate-out-bootstrap-network-configuration-files` - 1655 days ago (2021-09-11)
  - Modified networks.go to only have current networks
- `origin/ac-126-test-1-q-ben-nodes` - 1654 days ago (2021-09-13)
  - added QuentinBen network to networks.go for documentation test
- `origin/AC-126` - 1652 days ago (2021-09-15)
  - Added AWS Xeons to networks.go
- `origin/AC-125-define-implement-the-query-parameters-within-abci` - 1652 days ago (2021-09-15)
  - Merge pull request #15 from AccumulateNetwork/AC-195-query-data
- `origin/ethan-work` - 1646 days ago (2021-09-21)
  - feat: use cobra and move main
- `origin/AC-286-txhash-support` - 1646 days ago (2021-09-21)
  - API and proxy refactor
- `origin/AC-128-dockerfile` - 1645 days ago (2021-09-22)
  - update Dockerfile
- `origin/AC-250-basic-router` - 1644 days ago (2021-09-22)
  - merged with tendermint-0.35.0-rc1
- `origin/AC-250-basic-router-tendermint-update` - 1644 days ago (2021-09-22)
  - merged with tendermint-0.35.0-rc1
- `origin/test-target` - 1641 days ago (2021-09-26)
  - Merge pull request #25 from AccumulateNetwork/test-1
- `origin/test-2` - 1641 days ago (2021-09-26)
  - .
- `origin/AC-265-load-test` - 1640 days ago (2021-09-27)
  - feat: load test command
- `origin/AC-299-cleanup` - 1640 days ago (2021-09-27)
  - fix: revive app state from state DB
- `origin/AC-337-fix-gen-tx` - 1638 days ago (2021-09-29)
  - Merge pull request #49 from AccumulateNetwork/AC-335-unit-test-abci-accumulator
- `origin/AC-200-cli` - 1636 days ago (2021-10-01)
  - 32 bytes private key length
- `origin/AC-200-cli-queryfix` - 1636 days ago (2021-10-01)
  - fixed permissions
- `origin/AC-348-rename-gentx` - 1636 days ago (2021-09-30)
  - chore: rename GenTransaction => Transaction
- `origin/ethan-tx-payload` - 1633 days ago (2021-10-04)
  - cleanup tx payload marshalling
- `origin/fix-state-db-test` - 1631 days ago (2021-10-06)
  - fix: make state db consistency test a real boy
- `origin/bugfix/AC-364-merged-with-AC-200` - 1631 days ago (2021-10-06)
  - removed function not used
- `origin/AC-188-anon-tx-history-subchains` - 1630 days ago (2021-10-07)
  - Update e2e_test.go
- `origin/go-mod-tidy` - 1629 days ago (2021-10-08)
  - ci: enforce go mod tidy
- `origin/dashboards` - 1628 days ago (2021-10-09)
  - Update main.yml
- `origin/AC-145` - 1623 days ago (2021-10-13)
  - feat: create and assign key sets and groups
- `origin/AC-422` - 1622 days ago (2021-10-15)
  - feat(relay): return deliver TX results
- `origin/AC-444` - 1617 days ago (2021-10-20)
  - test: enable consistency/consensus test
- `origin/AC-388-add-error-reporting-for-batch-dispatch` - 1615 days ago (2021-10-21)
  - merge with develop
- `origin/AC-463` - 1609 days ago (2021-10-28)
  - ci: deploy to 3.140.120.192
- `origin/ci-health` - 1608 days ago (2021-10-28)
  - ci: environment health check
- `origin/fix-to-allow-faucet-to-resolve-nonce` - 1607 days ago (2021-10-29)
  - Merge remote-tracking branch 'origin/develop' into fix-to-allow-faucet-to-resolve-nonce
- `origin/cli-default-url'` - 1603 days ago (2021-11-02)
  - feat: change default CLI URL
- `origin/test-AC-489` - 1602 days ago (2021-11-04)
  - Saving work
- `origin/AC-519-human-readable-output-for-tx` - 1590 days ago (2021-11-16)
  - added README.md
- `origin/AC-509-alt` - 1589 days ago (2021-11-17)
  - feat: custom protocol error type
- `origin/bugfix/AC-554-fix-chain-id-from-cli` - 1589 days ago (2021-11-16)
  - fixed get by chain id
- `origin/AC-509-work` - 1584 days ago (2021-11-22)
  - Saving work
- `origin/stub-create-token` - 1583 days ago (2021-11-23)
  - chore: update token issuance
- `origin/AC-574` - 1582 days ago (2021-11-23)
  - feat: directory node stub
- `origin/move-chain-queries` - 1581 days ago (2021-11-24)
  - chore(chain): move queries to their own file
- `origin/AC-479-index-entries-not-unique` - 1579 days ago (2021-11-27)
  - fix: duplicate entry
- `origin/AC-558-Create-Unbound-Key-Page` - 1579 days ago (2021-11-26)
  - Updated script 3.1
- `origin/AC-662_Rename_to_LiteTokenAccount` - 1575 days ago (2021-12-01)
  - MERGE
- `origin/AC-544_state_db_readability_part3` - 1574 days ago (2021-12-02)
  - Merge remote-tracking branch 'origin/develop' into AC-544_state_db_readability_part3
- `origin/AC-544_state_db_readability_part2b` - 1574 days ago (2021-12-02)
  - AC-544: Fix after merge
- `origin/AC-544_state_db_readability_part2a` - 1568 days ago (2021-12-08)
  - AC-544: Fixed test code
- `origin/enable-tests-in-ci` - 1566 days ago (2021-12-10)
  - test: enable all tests in CI
- `origin/AC-697-protocol-recognition-when-lite-token-account-has-credits` - 1564 days ago (2021-12-12)
  - feat: added ability to fund a lite account with credits that doesn't yet exist.
- `origin/AC-542-Unix-socket` - 1561 days ago (2021-12-15)
  - remove tmpfile
- `origin/deploy-aws-ecr` - 1556 days ago (2021-12-20)
  - feat: deploy images to AWS ECR
- `origin/run-ci-on-gitlab` - 1547 days ago (2021-12-28)
  - ci: run most jobs on GitLab's infrastructure
- `origin/AC-639-goSDK` - 1540 days ago (2022-01-05)
  - added builder types for txns and key manager to support a signer interface for preparing txns
- `origin/query-direct` - 1539 days ago (2022-01-06)
  - fix: query direct
- `origin/AC-749-lite-data-chain` - 1533 days ago (2022-01-12)
  - Merge remote-tracking branch 'origin/develop' into AC-749-lite-data-chain
- `origin/AC-634` - 1526 days ago (2022-01-19)
  - cli work and added 1 seed per bvn/dn
- `origin/AC-825` - 1524 days ago (2022-01-21)
  - feat: update validators from the node's key book
- `origin/AC-726` - 1522 days ago (2022-01-23)
  - (feat) create-validator running in a cli txn
- `origin/terraform-test` - 1517 days ago (2022-01-27)
  - ci: terraform
- `origin/AC-925` - 1517 days ago (2022-01-27)
  - attempt to mirror keys whenever they're updated
- `origin/another-attempt-at-terraform` - 1516 days ago (2022-01-29)
  - Saving work
- `origin/terraform-saving-work` - 1516 days ago (2022-01-29)
  - Saving work
- `origin/AC-778-Prometheus` - 1516 days ago (2022-01-29)
  - AC-778: prometheus configuaration files
- `origin/AC-630_basic-routing-framework` - 1514 days ago (2022-01-31)
  - AC-630: saving work
- `origin/AC-888-Architecture-Documentation` - 1513 days ago (2022-02-01)
  - Comments, revision suggestions, clarity touchups.
- `origin/sdk` - 1512 days ago (2022-02-02)
  - feat(sdk): generalize type generators for use with external SDKs
- `origin/AC-832-c-enums-generator` - 1510 days ago (2022-02-04)
  - updated gitignore
- `origin/AC-1031` - 1509 days ago (2022-02-04)
  - saving work
- `origin/AC-1061` - 1499 days ago (2022-02-15)
  - AC-1061: included separate 16 nodes tf files
- `origin/AC-1042-Elucidation` - 1494 days ago (2022-02-19)
  - fix merge issues
- `origin/on-board` - 1492 days ago (2022-02-22)
  - fix error if only the public key is present
- `origin/AC-1113` - 1492 days ago (2022-02-22)
  - AC-1113: ecs with ec2 Launch type
- `origin/AC-756_advanced_routing_framework` - 1490 days ago (2022-02-24)
  - AC-756: var rename
- `origin/AC-1118-add-func-to-return-a-range-of-chainids-state-hashes-and-states` - 1489 days ago (2022-02-24)
  - CHORE: comment out failing test to be addressed in another issue.
- `origin/AC-1119-add-func-to-return-a-range-of-chainids-state-hashes-and-states` - 1485 days ago (2022-03-01)
  - chore: lint fix
- `origin/stevenmasley/readme_edits` - 1485 days ago (2022-02-28)
  - Remove localhost explicit ip
- `origin/AC-757_ADI-directories` - 1478 days ago (2022-03-08)
  - AC-757: saving impl work
- `origin/AC-1085-burn-acme-to-buy-credits-dhb` - 1477 days ago (2022-03-09)
  - fixed e2e_test for credits
- `origin/AC-992_nest-key-pages-with-key-books` - 1477 days ago (2022-03-09)
  - AC-992: Saving work
- `origin/fct2acme` - 1476 days ago (2022-03-10)
  - added example of how to generate an RCD from a public key
- `origin/merkle-hash` - 1476 days ago (2022-03-09)
  - preallocate the hasher
- `origin/revert-c827f710` - 1471 days ago (2022-03-15)
  - Revert "Merge branch 'develop' into 'AC-1034-support-querying-lite-data-accounts-by-url-factom-jj'"
- `origin/AC-1034-support-querying-lite-data-accounts-by-url-factom-jj` - 1471 days ago (2022-03-15)
  - Merge branch 'develop' into 'AC-1034-support-querying-lite-data-accounts-by-url-factom-jj'
- `origin/AC-1086-aggregate-credit-burns-and-send-them-with-the-anchor` - 1470 days ago (2022-03-16)
  - update
- `origin/AC-1089-only-charge-the-full-fee-once-a-transaction-is-promoted` - 1468 days ago (2022-03-18)
  - feat(prot):AC-1089-fix-transaction cost for pending tx
- `origin/AC-972-rebased` - 1465 days ago (2022-03-21)
  - fix(prot): key page entries must be sha256 hashes
- `origin/AC-1089-only-charge-the-full-fee-once-a-transaction-is-promoted-latest` - 1465 days ago (2022-03-21)
  - feat(prot):AC-1089-fix-transaction cost for pending tx
- `origin/AC-1188-remove-faucet` - 1465 days ago (2022-03-21)
  - compile error fix
- `origin/AC-972-keys-must-be-hashed-with-sha-256-jj` - 1464 days ago (2022-03-22)
  - Merge branch 'AC-972-keys-must-be-hashed-with-sha-256-jj' of https://gitlab.com/accumulatenetwork/accumulate into AC-972-keys-must-be-hashed-with-sha-256-latest
- `origin/AC-1238` - 1464 days ago (2022-03-21)
  - saving work
- `origin/AC-1244` - 1463 days ago (2022-03-23)
  - chore(prot): use a sub-batch for the state manager
- `origin/AC-1193` - 1463 days ago (2022-03-23)
  - AC-778: modifications based on mr review
- `origin/AC-1246-same-key` - 1461 days ago (2022-03-25)
  - gosimports
- `origin/api-v3-spec` - 1457 days ago (2022-03-28)
  - Lookup key
- `origin/AC-1133` - 1456 days ago (2022-03-30)
  - add light client store functions and errors
- `origin/AC-1106-ethan` - 1455 days ago (2022-03-31)
  - example benchmark
- `origin/AC-1290-fix-lazy-loading-of-the-bpt-root-node` - 1454 days ago (2022-04-01)
  - fix(bpt) lint
- `origin/AC-1146-fix-validator-test` - 1453 days ago (2022-04-02)
  - test: fix validator test
- `origin/tests-and-stuff` - 1453 days ago (2022-04-01)
  - dfbjk
- `origin/AC-1080` - 1450 days ago (2022-04-05)
  - It works!!!
- `origin/revert-18df342b` - 1450 days ago (2022-04-05)
  - Revert "AC-778: modifications based on mr review"
- `origin/AC-1205-associate-different-signature-types-with-accounts-in-cli` - 1449 days ago (2022-04-06)
  - feat(cli):Closes AC-1205 support alternate signature types
- `origin/AC-983` - 1447 days ago (2022-04-08)
  - chore(bpt): Added documentation, cleaned up indexing
- `origin/bug-fix-synth-receipts` - 1445 days ago (2022-04-09)
  - fix: synth receipt sig generation
- `origin/playbooks-notebooks` - 1444 days ago (2022-04-11)
  - add more detail
- `origin/AC-1335-add-ecdsa-signature-support-cli` - 1442 days ago (2022-04-14)
  - feat(prot):Implements AC-1335-add-ecdsa-signature-support-cli
- `origin/AC-1106` - 1435 days ago (2022-04-20)
  - configure number of transactions for a block
- `origin/load-test` - 1435 days ago (2022-04-19)
  - Do some load testing
- `origin/AC-1338-key-parameter-needed` - 1434 days ago (2022-04-21)
  - fix(cli): correct command line help for transactions [AC-1338]
- `origin/work-on-state-buckets` - 1434 days ago (2022-04-21)
  - work on state buckets
- `origin/AC-1319-move-transaction-status-to-the-principal` - 1428 days ago (2022-04-27)
  - Draft:feat(prot):AC-1319-move-transaction-status-to-the-principal
- `origin/AC-1424-human-readable-output-crashes-on-sendTokens` - 1427 days ago (2022-04-28)
  - Merge remote-tracking branch 'origin/develop' into AC-1424-human-readable-output-crashes-on-sendTokens
- `origin/data-hash` - 1427 days ago (2022-04-27)
  - Hash write data differently
- `origin/disable-space-check` - 1423 days ago (2022-05-02)
  - Merge remote-tracking branch 'origin/develop' into disable-space-check
- `origin/AC-1350-flush-pending-writes` - 1407 days ago (2022-05-18)
  - Merge remote-tracking branch 'origin/develop' into AC-1350-flush-pending-writes
- `origin/AC-1398_dn-account-subnets` - 1400 days ago (2022-05-25)
  - Merge remote-tracking branch 'origin/AC-1489-tld' into AC-1398_dn-account-subnets
- `origin/AC-1420_query-anchored-dn-blocks` - 1399 days ago (2022-05-26)
  - func renames
- `origin/AC-1506-separate-api-backend` - 1395 days ago (2022-05-30)
  - AC-1506: separate API from ABCI
- `origin/performance-tweaks` - 1386 days ago (2022-06-08)
  - optimize
- `origin/AC-1695-structured-cache` - 1379 days ago (2022-06-15)
  - clarity
- `origin/AC-1761` - 1379 days ago (2022-06-15)
  - fix
- `origin/AC-1494-update-the-bpt-to-track-the-adis-of-accounts` - 1374 days ago (2022-06-20)
  - updates
- `origin/AC-1695-batch-adapter` - 1373 days ago (2022-06-21)
  - saving work
- `origin/AC-2148-data-model-chains` - 1361 days ago (2022-07-02)
  - AC-2148: index chains
- `origin/AC-2072-accumulate-wallet-daemon` - 1355 days ago (2022-07-08)
  - Feat: (walletd) Added walletd command to hold accumulate in memory to provide wallet services
- `origin/work-on-restore-snapshot` - 1355 days ago (2022-07-08)
  - saving work
- `origin/refactor-signatures` - 1351 days ago (2022-07-13)
  - AC-2230: refactor how signatures are tracked
- `origin/AC-1278-add-network-endpoints` - 1350 days ago (2022-07-14)
  - Client offline functions and type structs
- `origin/Rosatte-construction-funs` - 1350 days ago (2022-07-14)
  - Client offline functions and type structs
- `origin/memoize-url-join-path` - 1344 days ago (2022-07-20)
  - memoize URL.JoinPath
- `origin/immutable-values` - 1344 days ago (2022-07-20)
  - implement immutable database values
- `origin/get-record-from-parent` - 1343 days ago (2022-07-21)
  - get record from parent
- `origin/memoize-record-keys` - 1343 days ago (2022-07-21)
  - memoize keys
- `origin/AC-1760-factom-snapshot-integration-ps` - 1337 days ago (2022-07-27)
  - Fix (factom genesis) updated reporting, avoid masswive submissions by waiting after every 100 blocks
- `origin/java-sdk` - 1335 days ago (2022-07-29)
  - merge go & java generators + linted
- `origin/AC-2234-many-cli-subcommands-accept-more-parameters-than-they-should` - 1335 days ago (2022-07-28)
  - AC-2234 applied suggestions
- `origin/AC-1597-node-status-update` - 1329 days ago (2022-08-04)
  - more work
- `origin/AC-2807-add-debug-and-pprof-flags-to-run-dual` - 1329 days ago (2022-08-03)
  - merge with develop
- `origin/AC-2811` - 1328 days ago (2022-08-05)
  - saving work
- `origin/AC-2810` - 1328 days ago (2022-08-05)
  - feat(prot): network address book [AC-2810]
- `origin/AC-2872-wip` - 1325 days ago (2022-08-08)
  - work on pending query
- `origin/DO-30` - 1324 days ago (2022-08-09)
  - cause a consensus failure
- `origin/debug-factom-genesis` - 1314 days ago (2022-08-19)
  - feat: (smt) Updates to caches
- `origin/x-move-pkg` - 1313 days ago (2022-08-20)
  - remove old files
- `origin/x-errors` - 1313 days ago (2022-08-20)
  - release errors package
- `origin/factom-import-nonce` - 1311 days ago (2022-08-22)
  - ci: disable MR check if labeled
- `origin/factom-import` - 1310 days ago (2022-08-23)
  - finished!
- `origin/factom-import-ps` - 1310 days ago (2022-08-22)
  - fix (factom) reduce block sizes and make entries unique
- `origin/rc2` - 1309 days ago (2022-08-24)
  - changed endpoint to beta
- `origin/faucet-break-consensus` - 1309 days ago (2022-08-24)
  - break consensus with faucet
- `origin/AC-3133-api-v3-initial` - 1298 days ago (2022-09-03)
  - feat(api/v3): node service
- `origin/AC-1710_snapshot-restore` - 1295 days ago (2022-09-07)
  - Working on StateSnapshot implementation
- `origin/AC-1529-stress-test` - 1295 days ago (2022-09-07)
  - loadtester script
- `origin/AC-3134-query-service` - 1295 days ago (2022-09-06)
  - feat(api/v3): query service implementation [AC-3134]
- `origin/AC-3172-event-service` - 1294 days ago (2022-09-08)
  - feat(api/v3): event service implementation [AC-3172]
- `origin/AC-3146-factom-testnet-prevent-replay` - 1294 days ago (2022-09-07)
  - feat(prot): prevent factoid replay [AC-3146]
- `origin/AC-3171-websocket-server` - 1293 days ago (2022-09-09)
  - saving work
- `origin/load-test-tweak` - 1292 days ago (2022-09-09)
  - saving work
- `origin/AC-3190-staking-approval` - 1288 days ago (2022-09-14)
  - AC-3190: construct framework for staking applications
- `origin/AC-3183-staking` - 1288 days ago (2022-09-14)
  - update
- `origin/AC-2434-revert-diff` - 1287 days ago (2022-09-15)
  - feat(node): shutdown if the DN stalls, reverted
- `origin/AC-1471-compose-txn` - 1287 days ago (2022-09-15)
  - compose transaction
- `origin/rc3` - 1287 days ago (2022-09-15)
  - bump database version
- `origin/set-sast-config-1` - 1287 days ago (2022-09-15)
  - Configure SAST in `.gitlab-ci.yml`, creating this file if it does not already exist
- `origin/DO-57-ci-fuzz` - 1286 days ago (2022-09-16)
  - ci n' stuff
- `origin/DO-23-generator-docs` - 1282 days ago (2022-09-20)
  - saving work
- `origin/DO-59-debug-decode` - 1281 days ago (2022-09-20)
  - feat(tool): debug decode tool
- `origin/AC-3211-implement-interface` - 1279 days ago (2022-09-23)
  - AC-3211: implement network interface
- `origin/remove-cli` - 1273 days ago (2022-09-28)
  - remove the cli
- `origin/AC-3226-staker-script` - 1269 days ago (2022-10-03)
  - AC-3226: commands to list and add staking accounts
- `origin/AC-3231-conn-mgr` - 1268 days ago (2022-10-03)
  - saving work
- `origin/AC-3230-service-router` - 1268 days ago (2022-10-03)
  - feat(api/v3): service router
- `origin/AC-3141-node-service` - 1268 days ago (2022-10-03)
  - feat(api/v3): node service implementation [AC-3141]
- `origin/DO-73-remove-node` - 1265 days ago (2022-10-07)
  - DO-73: remove node code
- `origin/cherry-pick-550b00c0` - 1262 days ago (2022-10-10)
  - merge: branch 'AC-3241-genesis-routing' into 'release-1.0'
- `origin/AC-3089-create-simulator-and-report-generator-for-staking` - 1262 days ago (2022-10-09)
  - fix (staking) bug in parameters in simulation
- `origin/DO-76-deactivate-validator` - 1261 days ago (2022-10-11)
  - update operator name
- `origin/simulator-tracing` - 1260 days ago (2022-10-12)
  - trace and profile
- `origin/AC-1468-generate-address` - 1254 days ago (2022-10-18)
  - Merge branch 'develop' of https://gitlab.com/accumulatenetwork/accumulate into AC-1468-generate-address
- `origin/AC-3236-add-memo-walletd` - 1254 days ago (2022-10-18)
  - Merge branch 'develop' of https://gitlab.com/accumulatenetwork/accumulate into AC-3236-add-memo-walletd
- `origin/AC-3192-cli-bug-cli-is-supposed-to-debit-credits-from-key-page-2-but-it-uses-key-page-1-instead` - 1253 days ago (2022-10-19)
  - cleanup
- `origin/AC-3124-encode-arbitrary-transaction-with-accumulate-walletd-jsonrpc-call` - 1253 days ago (2022-10-19)
  - encode-transaction
- `origin/AC-3185_ledger-sign-send-tokens` - 1253 days ago (2022-10-19)
  - Fixing import
- `origin/AC-3256-validate-walletd` - 1252 days ago (2022-10-20)
  - walletd validation script
- `origin/AC-2659-add-dual-mode-support-to-the-status-api-method` - 1251 days ago (2022-10-21)
  - key point
- `origin/AC-3185_ledger-sign-send-tokens-after-cli-split` - 1246 days ago (2022-10-26)
  - Merge branch 'main' into AC-3185_ledger-sign-send-tokens-after-cli-split
- `origin/AC-3283-scan-deposits` - 1244 days ago (2022-10-28)
  - feat(tool): scan for deposits in a block range
- `origin/pgp` - 1242 days ago (2022-10-30)
  - Test PGP 3
- `origin/activation` - 1237 days ago (2022-11-04)
  - lint
- `origin/AC-3083_java_generator_fixes` - 1233 days ago (2022-11-08)
  - use URL.toAccURL()
- `origin/DO-92-activation` - 1232 days ago (2022-11-08)
  - meh
- `origin/AC-3283-suggestions` - 1231 days ago (2022-11-10)
  - fix(scan deposits): handle case of no arguements
- `origin/show-vars` - 1226 days ago (2022-11-15)
  - .
- `origin/set-dependency-scanning-config-1` - 1226 days ago (2022-11-15)
  - Configure Dependency Scanning in `.gitlab-ci.yml`, creating this file if it does not already exist
- `origin/poc-advanced-auth` - 1216 days ago (2022-11-25)
  - Advanced auth rules PoC
- `origin/AC-832-c-types-generator` - 1211 days ago (2022-11-30)
  - merge with main
- `origin/database-interface` - 1207 days ago (2022-12-04)
  - saving work
- `origin/generatel-schema` - 1207 days ago (2022-12-03)
  - saving work
- `origin/pre-AC-3329` - 1204 days ago (2022-12-07)
  - Merge branch 'AC-3319-remove-old-sim' into pre-AC-3329
- `origin/enable-result-pipelines` - 1204 days ago (2022-12-06)
  - ci
- `origin/3148-executor-version` - 1198 days ago (2022-12-13)
  - Remove v2 for now
- `origin/get-original-value` - 1174 days ago (2023-01-05)
  - Fix MerkleManager.DidAddHashes [#3155]
- `origin/pre-3156` - 1170 days ago (2023-01-10)
  - Merge branches 'merge-1.0.2' and '3148-decouple-abci-from-executor' into pre-3156
- `origin/api-gold-file` - 1170 days ago (2023-01-09)
  - rename
- `origin/explorer-proxy` - 1169 days ago (2023-01-11)
  - Add a reverse proxy for the explorer
- `origin/debug-sign-address` - 1162 days ago (2023-01-18)
  - Add a tool for exporting an address
- `origin/3151-regenesis` - 1156 days ago (2023-01-24)
  - Recalculate hashes
- `origin/AC-3132-all-api` - 1148 days ago (2023-02-01)
  - AC-3132: API v3
- `origin/3199-update-create-account` - 1139 days ago (2023-02-09)
  - Update create account executors [#3199, 3]
- `origin/3199-update-create-identity` - 1139 days ago (2023-02-09)
  - Update create identity executor [#3199, 2]
- `origin/3199-new-txn-exec-framework` - 1139 days ago (2023-02-09)
  - New transaction executor framework [#3199]
- `origin/gold` - 1115 days ago (2023-03-06)
  - Gold file stuff
- `origin/work-on-executor-consistency` - 1115 days ago (2023-03-06)
  - Update gold file
- `origin/39-dn-follower-node` - 1106 days ago (2023-03-15)
  - Merge remote-tracking branch 'origin/main' into 39-dn-follower-node
- `origin/work-on-api` - 1106 days ago (2023-03-14)
  - fix
- `origin/separate-daemon` - 1102 days ago (2023-03-19)
  - tweak
- `origin/bpt2` - 1101 days ago (2023-03-20)
  - working on bpt
- `origin/3267-integrate-bpt` - 1100 days ago (2023-03-21)
  - oops
- `origin/for-staking-api-v3` - 1098 days ago (2023-03-23)
  - Merge branch '3274-support-secondary-services' into for-staking-api-v3
- `origin/3266-bpt-data-model` - 1097 days ago (2023-03-24)
  - Implement basic data model for BPT [#3266]
- `origin/light` - 1093 days ago (2023-03-28)
  - tweaks
- `origin/4-staking-scaffolding-support` - 1092 days ago (2023-03-29)
  - gosimports and cleanup
- `origin/bpt3` - 1091 days ago (2023-03-29)
  - BPT3
- `origin/bridge-refunds` - 1079 days ago (2023-04-11)
  - calculate refunds for the bridge
- `origin/3322-fix-sim` - 1036 days ago (2023-05-24)
  - Move message queue to the node [#3322]
- `origin/multi-network-node` - 1022 days ago (2023-06-07)
  - saving work
- `origin/testnet-revert` - 1022 days ago (2023-06-07)
  - fix for peer tracking
- `origin/write-batch` - 996 days ago (2023-07-03)
  - saving work
- `origin/sphereon/kotlin-mp-sdk` - 992 days ago (2023-07-07)
  - Implementing templates for Kotlin MPP code generator
- `origin/fix-stall-script` - 946 days ago (2023-08-22)
  - Script to fix stall
- `origin/3384-database-sync-tool-updates` - 944 days ago (2023-08-24)
  - go fmt, added assets to test, added print of fix file
- `origin/dbrepair-read-bug` - 943 days ago (2023-08-25)
  - fix read bug
- `origin/debug-flaky` - 940 days ago (2023-08-28)
  - Run tests with RR
- `origin/reset-consensus-script` - 939 days ago (2023-08-29)
  - Reset consensus script
- `origin/3379-better-anchor-healing-parallel` - 932 days ago (2023-09-05)
  - parallelize queries of peers
- `origin/api-debug` - 927 days ago (2023-09-10)
  - Better error handling
- `origin/files` - 918 days ago (2023-09-18)
  - Fix dbrepair
- `origin/serve-db` - 918 days ago (2023-09-18)
  - Merge branch '3390-update-dbrepair-to-add-missing-entries-alone' into serve-db
- `origin/sign-image` - 914 days ago (2023-09-23)
  - CI
- `origin/enable-some-tests` - 882 days ago (2023-10-24)
  - Enable some tests
- `origin/mexc` - 832 days ago (2023-12-14)
  - Merge branch 'release-1.2' into mexc
- `origin/new-config` - 816 days ago (2023-12-29)
  - Use tendermint dispatcher
- `origin/3539-unsigned-test-vectors` - 809 days ago (2024-01-05)
  - merge with main
- `origin/3495-bpt-backwards-compat` - 771 days ago (2024-02-13)
  - Temporarily revert BPT key change [#3495]
- `origin/foo` - 761 days ago (2024-02-22)
  - asdf
- `origin/3565-sign-typed-data-encoding` - 749 days ago (2024-03-06)
  - fix linting issues
- `origin/golangci-repro-4482` - 742 days ago (2024-03-13)
  - Minimal repro
- `origin/failure-chain-poc` - 733 days ago (2024-03-22)
  - Failure chain PoC
- `origin/acc-db` - 714 days ago (2024-04-10)
  - Custom db
- `origin/proxy-data-entries` - 694 days ago (2024-04-30)
  - Proxy data entries
- `origin/3589-code-intelligence` - 671 days ago (2024-05-22)
  - Try renaming
- `origin/3600-fee-schedule-consistency` - 669 days ago (2024-05-25)
  - added deprecation variable
- `origin/3570-generalize-pki-types` - 663 days ago (2024-05-31)
  - run gosimports
- `origin/3588-blockchain-specific-key-value-database` - 656 days ago (2024-06-06)
  - update
- `origin/database-benchmark-index` - 644 days ago (2024-06-19)
  - Benchmark index methods
- `origin/database-no-mutex` - 644 days ago (2024-06-19)
  - Avoid mutex
- `origin/database` - 638 days ago (2024-06-25)
  - Meh
- `origin/3617-instrumentation-followup` - 631 days ago (2024-07-02)
  - Meh
- `origin/3565-eip712-ethan` - 622 days ago (2024-07-10)
  - Implement eth_signTypedData_v4
- `origin/eip712-mask` - 621 days ago (2024-07-11)
  - Use masks to differentiate between versions of a struct
- `origin/3621-csv2yaml` - 607 days ago (2024-07-26)
  - added enum typedef generator
- `origin/3626-expose-simulator` - 561 days ago (2024-09-10)
  - added logging exports
- `origin/hashed-time-locks` - 559 days ago (2024-09-12)
  - Prepare for HTLCs
- `origin/3623-backport` - 541 days ago (2024-09-30)
  - Backport various fixes [#3623]
- `origin/3616-migrate-block-ledger` - 524 days ago (2024-10-17)
  - Implement indexing service
- `origin/partial-merkle-tree` - 421 days ago (2025-01-28)
  - Update of comments
- `origin/3644-csharp` - 384 days ago (2025-03-05)
  - tst
- `origin/3640-mining-support` - 371 days ago (2025-03-19)
  - wip to fix cycle errors
- `origin/3640-mining-support-hmm` - 369 days ago (2025-03-21)
  - wip to fix cycle errors

---

## Deletion Recommendations

### Summary

- **Safe to delete immediately**: 0 (stale merged branches)
- **Can delete after verification**: 0 (recently merged branches)
- **Likely can delete**: 272 (very old unmerged branches)
- **Needs review**: 80 (stale unmerged + old releases)
- **Total potential deletions**: 272

### Priority Deletion List

#### Phase 1: Safe Immediate Deletion (Stale Merged)


#### Phase 2: After Verification (Recently Merged)


#### Phase 3: Very Old Unmerged (Review First)

- 3148-executor-version
- 3151-regenesis
- 3199-new-txn-exec-framework
- 3199-update-create-account
- 3199-update-create-identity
- 3266-bpt-data-model
- 3267-integrate-bpt
- 3322-fix-sim
- 3379-better-anchor-healing-parallel
- 3384-database-sync-tool-updates
- 3495-bpt-backwards-compat
- 3539-unsigned-test-vectors
- 3565-eip712-ethan
- 3565-sign-typed-data-encoding
- 3570-generalize-pki-types
- 3588-blockchain-specific-key-value-database
- 3589-code-intelligence
- 3600-fee-schedule-consistency
- 3616-migrate-block-ledger
- 3617-instrumentation-followup
- 3621-csv2yaml
- 3623-backport
- 3626-expose-simulator
- 3640-mining-support
- 3640-mining-support-hmm
- 3644-csharp
- 39-dn-follower-node
- 4-staking-scaffolding-support
- AC-1031
- AC-1034-support-querying-lite-data-accounts-by-url-factom-jj
- AC-1042-Elucidation
- AC-1061
- AC-1080
- AC-1085-burn-acme-to-buy-credits-dhb
- AC-1086-aggregate-credit-burns-and-send-them-with-the-anchor
- AC-1089-only-charge-the-full-fee-once-a-transaction-is-promoted
- AC-1089-only-charge-the-full-fee-once-a-transaction-is-promoted-latest
- AC-1106
- AC-1106-ethan
- AC-1113
- AC-1118-add-func-to-return-a-range-of-chainids-state-hashes-and-states
- AC-1119-add-func-to-return-a-range-of-chainids-state-hashes-and-states
- AC-1133
- AC-1146-fix-validator-test
- AC-1188-remove-faucet
- AC-1193
- AC-1205-associate-different-signature-types-with-accounts-in-cli
- AC-1238
- AC-1244
- AC-1246-same-key
- AC-125-define-implement-the-query-parameters-within-abci
- AC-126
- AC-1278-add-network-endpoints
- AC-128-dockerfile
- AC-1290-fix-lazy-loading-of-the-bpt-root-node
- AC-1319-move-transaction-status-to-the-principal
- AC-1335-add-ecdsa-signature-support-cli
- AC-1338-key-parameter-needed
- AC-1350-flush-pending-writes
- AC-1398_dn-account-subnets
- AC-1420_query-anchored-dn-blocks
- AC-1424-human-readable-output-crashes-on-sendTokens
- AC-145
- AC-1468-generate-address
- AC-1471-compose-txn
- AC-1494-update-the-bpt-to-track-the-adis-of-accounts
- AC-1506-separate-api-backend
- AC-1529-stress-test
- AC-1597-node-status-update
- AC-1695-batch-adapter
- AC-1695-structured-cache
- AC-1710_snapshot-restore
- AC-1760-factom-snapshot-integration-ps
- AC-1761
- AC-188-anon-tx-history-subchains
- AC-200-cli
- AC-200-cli-queryfix
- AC-206_Refactor_Network
- AC-2072-accumulate-wallet-daemon
- AC-2148-data-model-chains
- AC-2234-many-cli-subcommands-accept-more-parameters-than-they-should
- AC-2434-revert-diff
- AC-250-basic-router
- AC-250-basic-router-tendermint-update
- AC-263-separate-out-bootstrap-network-configuration-files
- AC-265-load-test
- AC-2659-add-dual-mode-support-to-the-status-api-method
- AC-2807-add-debug-and-pprof-flags-to-run-dual
- AC-2810
- AC-2811
- AC-286-txhash-support
- AC-2872-wip
- AC-299-cleanup
- AC-3083_java_generator_fixes
- AC-3089-create-simulator-and-report-generator-for-staking
- AC-3124-encode-arbitrary-transaction-with-accumulate-walletd-jsonrpc-call
- AC-3132-all-api
- AC-3133-api-v3-initial
- AC-3134-query-service
- AC-3141-node-service
- AC-3146-factom-testnet-prevent-replay
- AC-3171-websocket-server
- AC-3172-event-service
- AC-3183-staking
- AC-3185_ledger-sign-send-tokens
- AC-3185_ledger-sign-send-tokens-after-cli-split
- AC-3190-staking-approval
- AC-3192-cli-bug-cli-is-supposed-to-debit-credits-from-key-page-2-but-it-uses-key-page-1-instead
- AC-3211-implement-interface
- AC-3226-staker-script
- AC-3230-service-router
- AC-3231-conn-mgr
- AC-3236-add-memo-walletd
- AC-3256-validate-walletd
- AC-3283-scan-deposits
- AC-3283-suggestions
- AC-337-fix-gen-tx
- AC-348-rename-gentx
- AC-388-add-error-reporting-for-batch-dispatch
- AC-422
- AC-444
- AC-463
- AC-479-index-entries-not-unique
- AC-509-alt
- AC-509-work
- AC-519-human-readable-output-for-tx
- AC-542-Unix-socket
- AC-544_state_db_readability_part2a
- AC-544_state_db_readability_part2b
- AC-544_state_db_readability_part3
- AC-558-Create-Unbound-Key-Page
- AC-574
- AC-630_basic-routing-framework
- AC-634
- AC-639-goSDK
- AC-662_Rename_to_LiteTokenAccount
- AC-697-protocol-recognition-when-lite-token-account-has-credits
- AC-726
- AC-749-lite-data-chain
- AC-756_advanced_routing_framework
- AC-757_ADI-directories
- AC-778-Prometheus
- AC-825
- AC-832-c-enums-generator
- AC-832-c-types-generator
- AC-888-Architecture-Documentation
- AC-925
- AC-972-keys-must-be-hashed-with-sha-256-jj
- AC-972-rebased
- AC-983
- AC-992_nest-key-pages-with-key-books
- DO-23-generator-docs
- DO-30
- DO-57-ci-fuzz
- DO-59-debug-decode
- DO-73-remove-node
- DO-76-deactivate-validator
- DO-92-activation
- Rosatte-construction-funs
- ac-126-test-1-q-ben-nodes
- acc-db
- activation
- add-chain-validation-engine
- add-validator-opcodes
- another-attempt-at-terraform
- api-debug
- api-gold-file
- api-v3-spec
- bpt2
- bpt3
- bridge-refunds
- bug-fix-synth-receipts
- bugfix/AC-364-merged-with-AC-200
- bugfix/AC-554-fix-chain-id-from-cli
- cherry-pick-550b00c0
- ci-health
- cli-default-url'
- dashboards
- data-hash
- database
- database-benchmark-index
- database-interface
- database-no-mutex
- dbrepair-read-bug
- debug-factom-genesis
- debug-flaky
- debug-sign-address
- deploy-aws-ecr
- disable-space-check
- eip712-mask
- enable-result-pipelines
- enable-some-tests
- enable-tests-in-ci
- ethan-tx-payload
- ethan-work
- expand-jsonrpc-api
- explorer-proxy
- factom-import
- factom-import-nonce
- factom-import-ps
- failure-chain-poc
- faucet-break-consensus
- fct2acme
- files
- fix-stall-script
- fix-state-db-test
- fix-to-allow-faucet-to-resolve-nonce
- foo
- for-staking-api-v3
- generatel-schema
- get-original-value
- get-record-from-parent
- go-mod-tidy
- golangci-repro-4482
- gold
- hashed-time-locks
- immutable-values
- java-sdk
- light
- load-test
- load-test-tweak
- memoize-record-keys
- memoize-url-join-path
- merkle-hash
- mexc
- move-chain-queries
- multi-network-node
- new-config
- on-board
- partial-merkle-tree
- performance-tweaks
- pgp
- playbooks-notebooks
- poc-advanced-auth
- pre-3156
- pre-AC-3329
- provide-helpers-for-internal-api
- proxy-data-entries
- query-direct
- rc2
- rc3
- refactor-signatures
- remove-cli
- reset-consensus-script
- revert-18df342b
- revert-c827f710
- run-ci-on-gitlab
- sdk
- separate-daemon
- serve-db
- set-dependency-scanning-config-1
- set-sast-config-1
- show-vars
- sign-image
- simulator-tracing
- sphereon/kotlin-mp-sdk
- stevenmasley/readme_edits
- stub-create-token
- terraform-saving-work
- terraform-test
- test-2
- test-AC-489
- test-target
- testnet-revert
- tests-and-stuff
- work-on-api
- work-on-executor-consistency
- work-on-restore-snapshot
- work-on-state-buckets
- write-batch
- x-errors
- x-move-pkg

---

## Actionable Next Steps

### Immediate Actions

1. **Review the active development branches** (31 branches marked [MERGED])
   - Most of these are recently merged and can be deleted once confirmed in dagbft-integration
   - Command to delete all recently merged branches:
   ```bash
   git push origin --delete \
     feature/issue-cleanup-64-issues \
     issue-3742-bullshark-ordering-fix \
     issue/dagbft-3815 \
     issue-3871-key-rotation \
     issue-3875-per-peer-vote-rate-limiting \
     feature/issue-3874 \
     issue-3872-timestamp-replay-protection \
     feature/issue-3863 \
     feature/issue-3857 \
     feature/issue-3858 \
     feature/issue-3854 \
     feature/issue-3855 \
     feature/issue-3850 \
     feature/issue-3853 \
     feature/issue-3851 \
     feature/issue-3852 \
     feature/issue-3849 \
     feature/issue-3844 \
     feature/issue-3845 \
     feature/issue-3843 \
     feature/issue-3847 \
     feature/issue-3846 \
     issue/dagbft-3821 \
     issue/dagbft-3819 \
     issue/dagbft-3818 \
     issue/dagbft-3820 \
     issue/dagbft-3822 \
     fix/lint-cleanup \
     3718-cometbft-analysis \
     issue-3742-fix-leader-chain-traversal
   ```

2. **Delete obsolete "essential" branches** that are actually stale:
   ```bash
   git push origin --delete \
     hotfix-main \
     master \
     not-develop \
     old-master \
     stepapp1-develop-patch-61548 \
     test-mainnet
   ```

3. **Delete old release branches** (keep only last 2-3 major releases):
   - Keep: release-1.4, release-1.3, release-1.2
   - Delete all others (cli-* and release-v0.* branches)

4. **Execute mass deletion of 272 very old branches**:
   - Script generated at `/tmp/delete-old-branches.sh`
   - These are all 1+ years old and never merged
   - Review the script before executing

### Conservative Approach (Recommended)

Execute deletions in phases:

**Phase 1 - Recently Merged Branches (Safe, ~30 branches)**
```bash
# Delete branches merged in last 30 days
git push origin --delete $(git branch -r --merged origin/dagbft-integration | grep -E 'feature/issue-|issue-3|issue/dagbft' | sed 's/  origin\///' | tr '\n' ' ')
```

**Phase 2 - Very Old Test/Debug Branches (Safe, ~100 branches)**
```bash
# Delete obvious test/experimental branches
git push origin --delete \
  test-2 test-target test-AC-489 test-mainnet \
  foo debug-flaky debug-sign-address debug-factom-genesis \
  ethan ethan-work ethan-tx-payload \
  not-develop old-master
```

**Phase 3 - Old Release Branches (Review first, ~25 branches)**
```bash
# Delete pre-1.0 releases
git push origin --delete \
  cli-v1.0.0-rc1 cli-v1.0.0-rc1.1 cli-v1.0.0-rc1.2 \
  cli-v1.0.0-rc1.2-debug cli-v1.0.0-rc2 \
  cli-0.7.0-beta cli-0.8.0-beta cli-0.8.1-beta cli-0.8.2-beta \
  cli-0.9.0-beta cli-0.9.1-beta cli-v0.4 \
  qa-0.7.0-beta release-v0.2 release-v0.3 release-v0.4 \
  release-v0.5.1 release-v0.6.0-rc0 release-v0.6.0-rc1 \
  release-v0.8.0-beta dev-1.0.2 hotfix-1.0.4 \
  regenesis-1.0 merge-release-v0.2
```

**Phase 4 - Remaining Very Old Branches (Execute script, ~150+ branches)**
```bash
# Review and execute the generated script
less /tmp/delete-old-branches.sh
# If satisfied:
bash /tmp/delete-old-branches.sh
```

### Branches to Keep

**Protected branches (10):**
- `main` - Production branch
- `dagbft-integration` - Active development branch
- `develop` - Legacy main branch (keep for reference)
- `aip-53-lxr-mining-base` - Important feature baseline
- `release-1.4` - Latest stable release
- `release-1.3` - Previous stable release
- `release-1.2` - Keep for reference
- `release-1.1` - Keep for reference
- `release-1.0` - Keep for reference

**Active work (1):**
- `3714-sdk-signature-docs` - Only unmerged branch with recent activity

**Under review (45 branches in 90-365 day range):**
- Review these manually to determine if work is still needed
- Key branches: 3691-mcp-server-for-accumulate, 3695-eliminate-need-for-observer, etc.

### Estimated Cleanup

- **Immediate safe deletion**: ~30 branches (recently merged)
- **Quick wins**: ~100 branches (obvious test/debug branches)
- **After review**: ~150 branches (very old, never merged)
- **Total potential cleanup**: ~280 branches (71% reduction)
- **Final count**: ~113 branches remaining

This would reduce from 393 branches to approximately 113 branches - a much more manageable repository.


# Restart and quorum-loss test — run 20261004T055059Z

HEAD `a38a66290`. Procedure: restart-test.py; the record of every step is chaos.log.

| step | result |
|---|---|
| A graceful whole-network restart | PASSED |
| B kill whole-network restart | PASSED |
| C one validator's store torn | PASSED |
| D two of BVN2's four lost, then restored | FAILED |
| E each validator restarted alone, twelve times | PASSED |

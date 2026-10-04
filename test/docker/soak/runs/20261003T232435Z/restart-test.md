# Restart and quorum-loss test — run 20261003T232435Z

HEAD `bb77f9222`. Procedure: restart-test.py; the record of every step is chaos.log.

| step | result |
|---|---|
| A graceful whole-network restart | PASSED |
| B kill whole-network restart | PASSED |
| C one validator's store torn | PASSED |
| D two of BVN2's four lost, then restored | PASSED |
| E each validator restarted alone, twelve times | PASSED |

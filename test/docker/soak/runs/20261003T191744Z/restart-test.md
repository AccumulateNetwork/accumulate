# Restart and quorum-loss test — run 20261003T191744Z

HEAD `949ac13b5`. Procedure: restart-test.py; the record of every step is chaos.log.

| step | result |
|---|---|
| A graceful whole-network restart | PASSED |
| B kill whole-network restart | PASSED |
| C one validator's store torn | FAILED |
| D two of BVN2's four lost, then restored | PASSED |

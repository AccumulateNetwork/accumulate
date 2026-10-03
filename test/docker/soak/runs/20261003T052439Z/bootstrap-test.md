# Follower bootstrap test — run 20261003T052439Z

HEAD `97b625a84`. Procedure: bootstrap-test.py (load 5 min; add 3 followers; wait synced + caught up; dwell 5 min; repeat).

| cycle | follower | partition | ACTIVE after (s) | synced after (s) | behind at dwell | drops in dwell |
|---|---|---|---|---|---|---|
| 1 | bvn1-fol1 | dn.acme | 60 | 182 | 0 | 0 |
| 1 | bvn1-fol1 | bvn | 60 | 182 | 0 | 0 |
| 1 | bvn2-fol1 | dn.acme | 60 | 182 | 0 | 0 |
| 1 | bvn2-fol1 | bvn | 50 | 171 | 0 | 0 |
| 1 | bvn3-fol2 | dn.acme | 60 | 182 | 0 | 0 |
| 1 | bvn3-fol2 | bvn | 60 | 192 | 0 | 0 |
| 2 | bvn1-fol1 | dn.acme | 60 | 181 | 0 | 0 |
| 2 | bvn1-fol1 | bvn | 50 | 181 | 0 | 0 |
| 2 | bvn2-fol1 | dn.acme | 60 | 182 | 0 | 0 |
| 2 | bvn2-fol1 | bvn | 60 | 192 | 0 | 0 |
| 2 | bvn3-fol2 | dn.acme | 60 | 182 | 0 | 0 |
| 2 | bvn3-fol2 | bvn | 50 | 182 | 0 | 0 |
| 3 | bvn1-fol1 | dn.acme | 61 | 182 | 0 | 0 |
| 3 | bvn1-fol1 | bvn | 50 | 172 | 0 | 0 |
| 3 | bvn2-fol1 | dn.acme | 61 | 182 | 0 | 0 |
| 3 | bvn2-fol1 | bvn | 71 | 202 | 0 | 0 |
| 3 | bvn3-fol2 | dn.acme | 61 | 182 | 0 | 0 |
| 3 | bvn3-fol2 | bvn | 81 | 212 | 0 | 0 |

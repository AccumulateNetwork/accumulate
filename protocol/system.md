# System Accounts

* `subnet` - Subnet ADI
* `subnet/operators` - Key book containing all network operator keys (synchronized)
* `subnet/validators` - Validator key book containing keys of the current validator set
* `subnet/ledger` - Internal ledger, tracks subnet and block status
  * `#minor-root` - Subnet root chain
  * `#minor-root-index` - Index for root chain
* `subnet/synthetic` - Synthetic transaction ledger, tracks status of produced and received synthetic transactions
* `subnet/anchors` - Anchor pool, collects anchors from other subnets
  * `#{id}-root` - Anchor chain for subnet `{id}`'s root chain
  * `#{id}-bpt` - Anchor chain for subnet `{id}`'s BPT
* `subnet/votes` - Records Tendermint votes
* `subnet/evidence` - Records Tendermint evidence
* `subnet/globals` - Global variables for the network (synchronized)
* `subnet/oracle` - ACME oracle value (currently DN-only)
* `subnet/network` - List of subnets and assignment of validators to subnets
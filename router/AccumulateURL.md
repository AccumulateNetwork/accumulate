#Accumulate URL 


##Actions [POST]
acc://[identityname]/[chain/path]?[action]&payload=[payload in hex]&sig=[signature in hex]&key=[key in hex]...

##Query [GET]
queries can be simple query paths.

acc://[identityname]/[chain/path]?query&...

| Query/Action | short hand | Description | Example |
| ----------------- | ---------------- | --------------- | --------------- |
|identity-create | idc | Create an identity chain. Requires a sponsor's signature for payment | acc://RedWagon?idc&sponsor=GreenRock&timestamp=[unix timestamp]&sig=[hex encoded signature]&key=[key used to sign] |
|token-url-create | turl | Add a token chain to an identity.  | acc://RedWagon/acc?turl&type=GreenRock/AccumulateTokenType&timestamp=[unix timestamp]&sig=[hex encoded signature]&key=[key used to sign] |
|token-tx| tx | Perform a token transaction | acc://RedWagon/acc?tx&tx=[hex encoded transaction per spec]&timestamp=[unix timestamp]&sig=[hex encoded signature]&key=[key used to sign]
|data-chain-create  |dcc | | | 
| data-entry | de | | |
|scratch-chain-create| scc | | | 
|scratch-entry | se | | |
|token-issue | ti | | |
|key-update | ku | | |
|deep-query | dq | Issue a transaction on the blockchain for a query that will provide a cryptographic receipt | |
|query | q | Issue a light weight simple query to a node | |

##Identity and Subidentities




##Transaction Types:
###Public Identity Chain

> acc://FA22de5NSG2FA2HmMaD4h8qSAZAJyztmmnwgLPghCQKoSekwYYct/identity?create=RedWagon&signature=
> acc://FA22de5NSG2FA2HmMaD4h8qSAZAJyztmmnwgLPghCQKoSekwYYct/data?entry=HEX?signature=

###Identity Management
####Creation


Create a subdomain Pizza under RedWagon

> acc://RedWagon/identity?create=RedWagon.Pizza

Create a new identity

> acc://RedWagon/identity?create=RadioFlyer

 

####Key Update
An identity can be updated to replace a public key.  The public key which will become invalid from
the current block forward, but can still be used to for resolving identity keys for entries prior to the current block 
height.  No transactions using the replaced key will be valid going forward.

> acc://RedWagon/identity?replace=PUBLICKEY1_HEX+PUBLICKEY2_HEX&signature=f97a65de43

####Delegate Identity
**TBD:** It is possible to delegate your identity to another identity for a given amount of time.  It will allow
others to sign transactions on your behalf.  Use with care
> acc://RedWagon/identity?delegate=

###Token URL Creation
CreateToken acc://RedWagon/create?token=
###Send Token

> acc://RedWagon/acc/send?amount=10.00?to=RadioFlyer

###Data Chain Creation

> acc://RedWagon/chain?create=

###Data Entry

###Scratch Chain Creation

###Scratch Entry 

###Issue Token




# Create Identity



CreateIdentity acc://RedWagon

* Who signs the identity?  Identities need to be bootstrapped. I.e. Someone needs to pay for it...
* Need to assign it to an initial public key?

CreateSubDomain acc://RedWagon.Pizza

acc://RedWagon/

https://localhost:12345/RedWagon/acc/query=balance

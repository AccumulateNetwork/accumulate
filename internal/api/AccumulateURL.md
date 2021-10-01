#Accumulate Data Structures

#Accumulate 
| data | Field Name | Description |
| ----------------- | ---------------- | --------------- | 
| **Header** |  | |
| varInt_F | Version | Version 2.  Higher numbers are currently rejected. Can safely be coded using 1 byte for the first 127 versions. |
| varInt_F | ChainIDLen | Length of chain identifier.  < 31 is a utf8 encoded chain name, 32 bytes in length is a sha256(chain name) Can be 0 if 
| 32 bytes | ChainID | This is the Chain which the author wants this Entry to go into |
| 32 bytes | ChainTypeID | This is the Chain which contains the formatting / rules upon which the entry payload is validated should be empty if data entry |
| 2 bytes | ExtIDs Size | Describes how many bytes required for the set of External IDs for this Entry.  Must be less than or equal to the Payload size.  Big endian. |
| **Payload** | | This is the data between the end of the Header and the end of the Content. |
| **External IDs** |  | This section is only interpreted and enforced if the External ID Size is greater than zero. |
| 2 bytes | ExtID element 0 length | This is the number of the following bytes to be interpreted as an External ID element.  Cannot be 0 length. |
| variable | ExtID 0 | This is the data for the first External ID. |
| 2 bytes | ExtID X | Size of the X External ID  |
| variable | ExtID X data | This is the Xth element.  The last byte of the last element must fall on the last byte specified ExtIDs Size in the header. |
| **Content** | | |
| variable | Entry Data | This is the unstructured part of the Entry.  It is all user specified data. |

Minimum empty Entry length: 35 bytes

Minimum empty first Entry with Chain Name of 1 byte: 38 bytes

Maximum Entry size: 10KiB + 35 bytes = 10275 bytes

Typical size recording the hash of a file with 200 letters of ExtID metadata: 1+32+2+2+200+32 = 269 bytes


#Accumulate URL

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

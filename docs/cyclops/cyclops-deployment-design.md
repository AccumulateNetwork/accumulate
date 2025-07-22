

Update the dcyclops deployment design with the following.  Then adjust the entire document with the literal steps that should be implemented in three scripts  defined n three md files.  
The actual scripts will be deployed from these md files to ~/accumulte-network/artifacts and executed from there.

Overview
We are having issues with keys agreeing between the networ json, the consensus section in the parition snapshots, and the priv_validator_key.json for the dn node and the priv_validatoar_key for the bvn node
So the intention is to follow these steps:

First Prep:  prepare the artifacts for ~/accumulate-network/artifacts with these 5 steps

1. create the unitfied snapshot cyclops-genesis.snap.  For now, we will assume we have this in the ~/accumulate-network/artifacts
2. create the keys that we need, which would be the two priv_validator_key.json files for the dn and the bvn.  Keep up with these two.  I suggest adding the partition to the filename (priv_validator_key_{dn or bvn0}.json and put these files in ~/accumulate-network/artifacts
3. update the cyclops_network.json with the public keys for the validator node (its dn and bvn public key from their respective priv_validator_key_{dn or bvn0}.json files
4.  Create the consensus.json files that will be put in the partition snapshots consensus_{dn or bvn}.json
5. Extract the cyclopse-genesis.snap with the network.json and the two consensus_{dn or bvn0}.json  files, using the right one for each partition snapshot

Second Deploy:  Assuming Prep was successful, deploy artifacts to the validator node:

1. remove any past deployed node at /tmp/cyclops/
2. create /tmp/cyclops/artifacts
3 copy the unified snapshot,  the priv_validator_key_{dn or bvn0} files,  the updated network.json, and the partition shapshots  to /tmp/cyclops/ 
4. the construction of the node configuraiton will be done using the artifacts collected in tmp/cyclops/artifacts
4 execute accumulated init node with the artifacts from /tmp/cyclops
6 Check and ensure that that toml files are generated and in the correct place, the priv_validator_key.json files are correct and in the righ tplace, the partition snapshots are in the right place

Third launch:  Properly launch the validator node

7. I believe this might be the accumulated init dual command.
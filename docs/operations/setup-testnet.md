# Setting up a TestNet

## Adding your key to dn.acme/operators

1. Import the bootstrap keys into a wallet:

   ```shell
   KEY=$(ssh kermit-bvn0.accumulate.defidevs.io 'cat /var/lib/docker/volumes/acc_kermit_bvn0/_data/priv_validator_key.json')
   accumulate key import private kermit_bvn0 <(echo $KEY)
   ```

2. Add a page with your key:

   ```shell
   accumulate page create dn.acme/operators <your key>
   accumulate tx sign <tx hash>
   ```

## Reset back to genesis

Run the `NUKE-STATE.sh` maintenance script on every node.
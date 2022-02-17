#!/bin/bash
#
# test case 1.1
#
# generate 100 lite token accounts
# server IP:Port needed
#
# set cli command and see if it exists
#

# Ensure the CLI and API are defined
REPO=$(git rev-parse --show-toplevel)
cd "${REPO}/scripts/matrix"
source build_cli.sh
source ensure_api.sh

# call our create id script 100 times

for i in {1..100}
do

   ID=`./cli_create_id.sh $ACC_API`

   # see if we got an id, if not, exit

   if [ $? -eq 0 ]; then
      # return the ID
      echo $ID
   else
     echo "Account creation failed"
     exit 1
   fi

done
exit 0

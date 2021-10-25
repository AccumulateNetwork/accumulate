#!/bin/bash
#
# test case 1.1
#
# generate 100 lite accounts
# server IP:Port needed
#
# check for command line parameters
if [ -z $1 ]; then
	echo "You must pass IP:Port for the server to use"
	exit 0
fi

# call our create id script 100 times

for i in {1..100}
do

   ID=`./cli_create_id.sh $1`

   # see if we got an id, if not, exit

   if [ -z $ID ]; then
     echo "Account creation failed"
     exit 0
   fi

   # return the ID

   echo $ID

done


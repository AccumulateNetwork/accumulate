#!/bin/bash
#
# test case 1.1
#
# generate 100 lite accounts
# server IP:Port needed
#
# set cli command and see if it exists
#
export cli=../../cmd/cli/cli

if [ ! -f $cli ]; then
	echo "cli command not found in ../cmd/cli, attempting to build"
        ./build_cli.sh
	if [ ! -f $cli ]; then
	        echo "cli command failed to build"
		exit 1
	fi
fi

if [ -z $1 ]; then
	echo "You must pass IP:Port for the server to use"
	exit 1
fi

# call our create id script 100 times

for i in {1..100}
do

   ID=`./cli_create_id.sh $1`

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

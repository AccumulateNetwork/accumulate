#!/bin/bash
#
# test case 6.7
#
# xfer funds between lite token accounts, one that has no funds to transfer
# ids, amount and server IP:Port needed
#
# set cli command and see if it exists
#
export cli=../../cmd/accumulate/accumulate

if [ ! -f $cli ]; then
        echo "accumulate command not found in ../../cmd/accumulate, attempting to build"
        ./build_cli.sh
        if [ ! -f $cli ]; then
           echo "accumulate command failed to build"
           exit 1
        fi
fi

# create some IDs and don't faucet either of them

id1=`cli_create_id.sh $1`
if [ $? -ne 0 ]; then
	echo "accumulate create id failed"
	exit 1
fi
id2=`cli_create_id.sh $1`
if [ $? -ne 0 ]; then
	echo "accumulate create id failed"
	exit 1
fi

#call our xfer script

./cli_xfer_tokens.sh $id1 $id2 $3 $4

if [ $? -ne 0 ]; then
	echo "accumulate xfer tokens failed"
	exit 1
fi
exit 0

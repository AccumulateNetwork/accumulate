#!/bin/bash
#
# test case 5.1
#
# fund lite token account
# id and server IP:Port needed
#
# set cli command and see if it exists
#
export cli=../../cmd/accumulate/accumulate

if [ ! -f $cli ]; then
        echo "cli command not found in ../../cmd/cli, attempting to build"
        ./build_cli.sh
        if [ ! -f $cli ]; then
           echo "cli command failed to build"
           exit 1
        fi
fi
# check for command line parameters
#
if [ -z $1 ]; then
	echo "You must pass an ID to be funded"
	exit 1
fi
if [ -z $2 ]; then
	echo "You must pass IP:Port for the server to use"
	exit 1
fi

# call our faucet script

TxID=`./cli_faucet.sh $1 $2`
if [ $? -ne 0 ]; then
	echo "cli faucet failed"
	exit 1
fi

# check transaction status

Status=`./cli_get_tx_status.sh $TxID $2`
if [ $? -ne 0 ]; then
	echo "Invalid status received from get tx"
	exit 1
fi

# get our balance

bal=`./cli_get_balance.sh $1 $2`
if [ $? -ne 0 ]; then
	echo "cli get balance failed"
	exit 1
fi

echo $bal
exit 0

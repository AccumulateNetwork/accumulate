#!/bin/bash
#
# test case 5.1
#
# fund lite account
# id and server IP:Port needed
#
# set cli command and see if it exists
#
export cli=../cmd/cli/cli

if [ ! -f $cli ]; then
	echo "cli command not found in ../cmd/cli, cd to ../cmd/cli and run go build"
	exit 0
fi
# check for command line parameters
#
if [ -z $1 ]; then
	echo "You must pass an ID to be funded"
	exit 0
fi
if [ -z $2 ]; then
	echo "You must pass IP:Port for the server to use"
	exit 0
fi

# call our faucet script

TxID=`./cli_faucet.sh $1 $2`

# check transaction status

Status=`./cli_get_tx_status.sh $TxID $2`

if [ ! -z $Status ]; then
	echo "Invalid status received from get tx"
fi

# get our balance

bal=`./cli_get_balance.sh $1 $2`

echo $bal

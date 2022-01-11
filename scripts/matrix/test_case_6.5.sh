#!/bin/bash
#
# test case 6.5
#
# check tx status - fund an account and query the transaction
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

# if IDs not entered on the command line, prompt for one and exit

if [ -z $1 ]; then
        echo "Usage: test_case_6.5.sh ID IPAddress:Port"
        exit 1
fi

# see if $1 is really an ID

id1=$1
size=${#id1}
if [ $size -lt 59 ]; then
        echo "Expected acc://<48 byte string>/ACME"
        exit 1
fi

if [ -z $2 ]; then
        echo "Usage: test_case_6.5.sh ID IPAddress:Port"
        exit 1
fi

# call our faucet script and get the txid

txid=`./cli_faucet.sh $id1 $2` 

if [ $? -ne 0 ]; then
	echo "cli faucet failed"
	exit 1
fi
# call our get tx status script to get the tx status

status=`./cli_get_tx_status.sh $txid $2`

if [ $? -ne 0 ]; then
	echo "cli get tx status failed"
	exit 1
fi
echo $status

exit 0

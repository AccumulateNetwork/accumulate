#!/bin/bash
#
# test case 6.5
#
# check tx status - fund an account and query the transaction
# id and server IP:Port needed
#
# check for command line parameters
#

# if IDs not entered on the command line, prompt for one and exit

if [ -z $1 ]; then
        echo "Usage: test_case_6.5.sh ID IPAddress:Port"
        exit 0
fi

# see if $1 is really an ID

id1=$1
size=${#id1}
if [ $size -lt 59 ]; then
        echo "Expected acc://<48 byte string>/ACME"
        exit 0
fi

if [ -z $2 ]; then
        echo "Usage: test_case_6.5.sh ID IPAddress:Port"
        exit 0
fi

# call our faucet script and get the txid

txid=`./cli_faucet.sh $id1 $2` 
txid=`echo $txid | /usr/bin/sed 's/"//g'`

# call our get tx status script to get the tx status

status=`./cli_get_tx_status.sh $txid $2`

echo $status


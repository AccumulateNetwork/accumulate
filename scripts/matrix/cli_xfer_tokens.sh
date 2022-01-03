#!/bin/bash
#
# This script uses the accumulate cli to create a transfer transaction
# The script expects two IDs, number of tokesn and server IP:Port to be passed in
#
# see if jq exist
#
j=`which jq`
if [ -z $j ]; then
        echo "jq is needed to get the error code"
        exit 1
fi

# if IDs not entered on the command line, prompt for one and exit

if [ -z $1 ]; then
	echo "Usage: cli_xfer_tokens.sh fromID toID numTokens <IPAddress:Port>"
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
	echo "Usage: cli_xfer_tokens.sh fromID toID numTokens <IPAddress:Port>"
	exit 1
fi

# see if $2 is really an ID

id2=$2
size=${#id2}
if [ $size -lt 59 ]; then
	echo "Expected acc://<48 byte string>/ACME"
	exit 1
fi

if [ -z $3 ]; then
	echo "Usage: cli_xfer_tokens.sh fromID toID numTokens <IPAddress:Port>"
	exit 1
fi

# is there enough of a balance in $1 to xfer to $2?

bal=`./cli_get_balance.sh $id1 $4`

if [ $? -ne 0 ]; then
	echo "get balance for first id failed"
	exit 1
fi

if [[ bal -lt $3 ]]; then
	echo "Not enough funds in $1 for requested xfer"
	exit 1
fi

# see if the IP address and port were entered on the command line
# issue the account get command for the specified ID to the specified server

if [ -z $4 ]; then
	txid1="$($cli tx create $id1 $id2 $3 -j 2>&1 > /dev/null)"
        if [ $? -eq 0 ]; then
	   txid=`echo $txid1 | $j .error`
        else
	   echo "accumulate tx create failed"
	   exit 1
        fi
else
	txid1="$($cli tx create $id1 $id2 $3 -s http://$4/v1 -j 2>&1 > /dev/null)"
        if [ $? -eq 0 ]; then
	   txid=`echo $txid1 | $j .error`
        else
	   echo "accumulate tx create failed"
	   exit 1
        fi
fi

bal=`./cli_get_balance.sh $id2 $4`

if [ $? -ne 0 ]; then
	echo "get balance for second id failed"
	exit 1
fi

# return the balance in id2

echo $bal 
exit 0

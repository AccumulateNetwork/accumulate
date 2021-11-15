#!/bin/bash
#
# This script uses the accumulate cli to fund an account
# The script expects an ID and server IP:Port to be passed in
#
# Use jq to parse the returned json information to get the transaction id
#
if [ ! -f /usr/bin/jq ]; then
	echo "jq must be installed to return the transaction ID"
	exit 0
fi

# if ID entered on the command line, prompt for one and exit

if [ -z $1 ]; then
	echo "Usage: cli_faucet.sh ID IPAddress:Port"
	exit 0
fi

# see if $1 is really an ID

id1=$1
size=${#id1}
if [ $size -lt 59 ]; then
        echo "Expected acc://<48 byte string>/ACME"
        exit 0
fi

# see if the IP address and port were entered on the command line
# issue the faucet command for the specified ID to the specified server

if [ -z $2 ]; then
   ID="$($cli faucet $id1 -j 2>&1 > /dev/null | jq .txid)"
else
   ID="$($cli faucet $id1 -s http://$2/v1 -j 2>&1 > /dev/null | jq .txid)"
fi

# return the transaction ID 

echo $ID 



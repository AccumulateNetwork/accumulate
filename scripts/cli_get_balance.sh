#!/bin/bash
#
# This script uses the accumulate cli to get the balance of an account
# The script expects an ID and server IP:Port to be passed in
#

# if ID entered on the command line, prompt for one and exit

if [ -z $1 ]; then
	echo "Usage: cli_get_balance.sh ID <IPAddress:Port>"
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

# issue the account get command for the specified ID to the specified server

if [ -z $2 ]; then
   bal="$($cli account get $id1 -j 2>&1 > /dev/null | jq .data.balance | sed 's/\"//g')"
else
   bal="$($cli account get $id1 -s http://$2/v1 -j 2>&1 > /dev/null | jq .data.balance | sed 's/\"//g')"
fi

# return the balance information

echo $bal 



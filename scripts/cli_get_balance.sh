#!/bin/bash
#
# This script uses the accumulate cli to get the balance of an account
# The script expects an ID and server IP:Port to be passed in
#

# if ID entered on the command line, prompt for one and exit

if [ -z $1 ]; then
	echo "Usage: cli_get_balance.sh ID IPAddress:Port"
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

if [ -z $2 ]; then
	echo "You must enter an IPAddress:Port for a server to generate an account"
	exit 0
fi

# issue the account get command for the specified ID to the specified server

bal=`$cli account get $id1 -s "http://$2/v1"`

# return the balance information

echo $bal | jq .data.balance | /usr/bin/sed 's/$//g'



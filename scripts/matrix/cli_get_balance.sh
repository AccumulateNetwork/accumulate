#!/bin/bash
#
# This script uses the accumulate cli to get the balance of an account
# The script expects an ID and server IP:Port to be passed in
#
# see if jq and sed exist
#
j=`which jq`
if [ -z $j ]; then
	echo "jq is needed to get balance"
	exit 1
fi

s=`which sed`
if [ -z $s ]; then
	echo "sed is needed to get balance"
	exit 1
fi


# if ID entered on the command line, prompt for one and exit

if [ -z $1 ]; then
	echo "Usage: cli_get_balance.sh ID <IPAddress:Port>"
	exit 1
fi

# see if $1 is really an ID

id1=$1
size=${#id1}
if [ $size -lt 59 ]; then
        echo "Expected acc://<48 byte string>/ACME"
        exit 1
fi

# see if the IP address and port were entered on the command line

# issue the account get command for the specified ID to the specified server

if [ -z $2 ]; then
	balnc="$($cli account get $id1 -j 2>&1 > /dev/null)"
        if [ $? -eq 0 ]; then
           bal=`echo $balnc | $j .data.balance | $s 's/\"//g'`
        else
	   echo "cli account get failed"
	   exit 1
       fi
else
	balnc="$($cli account get $id1 -s http://$2/v1 -j 2>&1 > /dev/null)"
        if [ $? -eq 0 ]; then
       	   bal=`echo $balnc | $j .data.balance | $s 's/\"//g'`
        else
	   echo "cli account get failed"
	   exit 1
        fi
fi

# return the balance information

echo $bal 
exit 0


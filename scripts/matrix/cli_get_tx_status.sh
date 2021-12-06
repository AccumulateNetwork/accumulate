#!/bin/bash
#
# This script uses the accumulate cli to get the status of a transaction
# The script expects an ID and server IP:Port to be passed in
#
# see if jq and sed exist
#
j=`which jq`
if [ -z $j ]; then
        echo "jq is needed to get status"
        exit 1
fi

s=`which sed`
if [ -z $s ]; then
        echo "sed is needed to get status"
        exit 1
fi

# if ID entered on the command line, prompt for one and exit

if [ -z $1 ]; then
	echo "Usage: cli_get_tx_status.sh ID IPAddress:Port"
	exit 1
fi

# see if $1 is really an ID

id1=$1
size=${#id1}
if [ $size -lt 48 ]; then
        echo "Expected 48 byte string"
        exit 1
fi

# see if the IP address and port were entered on the command line
# issue the tx get command for the specified ID to the specified server

if [ -z $2 ]; then
	StatusTX="$($cli tx get $id1 2>&1 > /dev/null)"
        if [ $? -eq 0 ]; then
	   Status=`echo $StatusTX | $j .status.code | $s 's/\"/g'`
       else
	   echo "cli tx get failed"
	   exit 1
       fi
else
	StatusTX="$($cli tx get $id1 -s http://$2/v1 2>&1 > /dev/null)"
        if [ $? -eq 0 ]; then
	   Status=`echo $StatusTX | $j .status.code | $s 's/\"/g'`
       else
	   echo "cli tx get failed"
	   exit 1
       fi
fi

# return the status information

echo $Status 
exit 0


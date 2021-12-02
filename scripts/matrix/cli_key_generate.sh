#!/bin/bash 
#
# This script uses the accumulate cli to generate a key
# The script expects a key name and server IP:Port to be passed in
#
# see if jq and sed exist
#
j=`which jq`
if [ -z $j ]; then
        echo "jq is needed to get key"
        exit 0
fi

s=`which sed`
if [ -z $s ]; then
        echo "sed is needed to get key"
        exit 0
fi

if [ -z $cli ]; then
	cli=../cmd/cli/cli
fi

# if key name not entered on the command line, prompt for one and exit 

if [ -z $1 ]; then
	echo "Usage: cli_faucet.sh keyname IPAddress:Port"
	exit 0
fi

# see if the IP address and port were entered on the command line
# issue the key generate command for the specified key name to the specified server

if [ -z $2 ]; then
   key="$($cli key generate $1 -j 2>&1 > /dev/null | $j .publicKey | $s 's/\"//g')"
else
   key="$($cli key generate $1 -s http://$2/v1 -j 2>&1 > /dev/null | $j .publicKey | $s 's/\"//g')"
fi

# return the key

echo $key 



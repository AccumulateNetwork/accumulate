#!/bin/bash
#
# This script uses the accumulate cli to generate a key
# The script expects a key name and server IP:Port to be passed in
#
# Use jq to parse the returned json information to get the public key
#
if [ ! -f /usr/bin/jq ]; then
	echo "jq must be installed to return the key"
	exit 0
fi

# if key name not entered on the command line, prompt for one and exit 

if [ -z $1 ]; then
	echo "Usage: cli_faucet.sh keyname IPAddress:Port"
	exit 0
fi

# see if the IP address and port were entered on the command line
# issue the key generate command for the specified key name to the specified server

if [ -z $2 ]; then
   key="$($cli key generate $1 -j 2>&1 > /dev/null | jq .publicKey | sed 's/\"//g')"
else
   key="$($cli key generate $1 -s http://$2/v1 -j 2>&1 > /dev/null | jq .publicKey | sed 's/\"//g')"
fi

# return the key

echo $key 



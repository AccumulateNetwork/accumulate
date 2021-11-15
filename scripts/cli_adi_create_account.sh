#!/bin/bash
#
# This script uses the accumulate cli to create an ADI account
# The script expects a lite account, an account name, a key name and server IP:Port to be passed in
#
# Use jq to parse the returned json information to get the public key
#
if [ ! -f /usr/bin/jq ]; then
	echo "jq must be installed to return the key"
	exit 0
fi

# if lite account, account name, and key name not entered on the command line, prompt and exit 

if [ -z $1 ] || [ -z $2 ] || [ -z $3 ] || [ -z $4 ]; then
	echo "Usage: cli_adi_create_account.sh liteacct acctname keyname IPAddress:Port"
	exit 0
fi

# see if the IP address and port were entered on the command line
# issue the key generate command for the specified key name to the specified server

if [ -z $2 ]; then
   key="$($cli adi create $1 $2 $3 -j 2>&1 > /dev/null | /usr/bin/jq .hash | /usr/bin/sed 's/\"//g')"
else
   key="$($cli adi create $1 $2 $3 -s http://$4/v1 -j 2>&1 > /dev/null | /usr/bin/jq .hash | /usr/bin/sed 's/\"//g')"
fi

# return the key

echo $key 



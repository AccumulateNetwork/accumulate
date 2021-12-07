#!/bin/bash 
#
# This script uses the accumulate cli to create an ADI account
# The script expects a lite account, an account name, a key name and server IP:Port to be passed in
#
# Use jq to parse the returned json information to get the public key
#
j=`which jq`
if [ -z $j ]; then
	echo "jq must be installed to return the key"
	exit 1
fi

s=`which sed`
if [ -z $s ]; then
	echo "sed must be installed to return the key"
	exit 1
fi

if [ -z $cli ]; then
	cli=../../cmd/cli/cli
fi

# if lite account, account name, and key name not entered on the command line, prompt and exit 

if [ -z $1 ] || [ -z $2 ] || [ -z $3 ] || [ -z $4 ]; then
	echo "Usage: cli_adi_create_account.sh liteacct acctname keyname IPAddress:Port"
	exit 0
fi

# see if the IP address and port were entered on the command line
# issue the adi create command for the specified key name to the specified server

keyn="$($cli adi create $1 $2 $3 -s http://$4/v1 -j 2>&1 > /dev/null)"
if [ $? -eq 0 ]; then
	key=`echo $keyn | $j .hash | $s 's/\"//g'`
else
	echo "cli adi create failed"
	exit 1
fi

echo $key 
exit 0


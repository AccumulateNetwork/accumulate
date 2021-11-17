#!/bin/bash
#
# This script uses the accumulate cli to generate an account ID
#
# look for jq and sed
#
j=`which jq`
if [ -z $j ]; then
	echo "jq not found, needed to get account name"
	exit 0
fi
s=`which sed`
if [ -z $s ]; then
	echo "jq not found, needed to get account name"
	exit 0
fi
#
# issue the account generate command to the specified server

if [ -z $1 ]; then
	acc="$($cli account generate -j 2>&1 > /dev/null | $j .name | $s 's/\"//g')"
else
	acc="$($cli account generate -j -s http://$1/v1 2>&1 > /dev/null | $j .name | $s 's/\"//g')"
fi

# return the generated ID 

echo $acc



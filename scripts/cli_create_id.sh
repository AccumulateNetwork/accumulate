#!/bin/bash
#
# This script uses the accumulate cli to generate an account ID
#
# if no command line parameter entered, prompt 
#
# issue the account generate command to the specified server

if [ -z $1 ]; then
	acc="$($cli account generate -j 2>&1 > /dev/null | jq .name | sed 's/\"//g')"
else
	acc="$($cli account generate -j -s http://$1/v1 2>&1 > /dev/null | jq .name | sed 's/\"//g')"
fi

# return the generated ID 

echo $acc



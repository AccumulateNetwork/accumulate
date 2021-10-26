#!/bin/bash
#
# This script uses the accumulate cli to generate an account ID
#
# if no command line parameter entered, prompt for one and exit
#
if [ -z $1 ]; then
	echo "You must enter an IPAddress:Port for a server to generate an account"
	exit 0
fi

# issue the account generate command to the specified server

ID=`$cli account generate -s "http://$1/v1"`

# return the generated ID 

echo $ID



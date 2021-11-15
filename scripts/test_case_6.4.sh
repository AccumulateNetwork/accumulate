#!/bin/bash
#
# test case 6.4
#
# xfer funds between ADI accounts
# ids, amount and server IP:Port needed
#
# set cli command and see if it exists
#
export cli=../cmd/cli/cli

if [ ! -f $cli ]; then
	echo "cli command not found in ../cmd/cli, cd to ../cmd/cli and run go build"
	exit 0
fi
# check for command line parameters
#

# if IDs not entered on the command line, prompt for one and exit

if [ -z $1 ]; then
        echo "Usage: test_case_6.4.sh fromID toID numTokens IPAddress:Port"
        exit 0
fi

# see if $1 is really an ADI account

id1=$1
if [ ${id1:0:6} != "acc://" ]; then
        echo "Expected ADI account acc://<string>"
        exit 0
fi

if [ -z $2 ]; then
        echo "Usage: test_case_6.4.sh fromID toID numTokens IPAddress:Port"
        exit 0
fi

# see if $2 is really an ADI account

id2=$2
if [ ${id2:0:6} != "acc://" ]; then
        echo "Expected ADI account acc://<string>"
        exit 0
fi

if [ -z $3 ]; then
        echo "Usage: test_case_6.4.sh fromID toID numTokens IPAddress:Port"
        exit 0
fi

# call our xfer script

./cli_xfer_tokens.sh $id1 $id2 $3 $4


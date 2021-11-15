#!/bin/bash
#
# test case 2.1
#
# create an adi "redwagon" sponsored by a lite account
# server IP:Port needed unless defaulting to localhost
#
# set cli command and see if it exists
#
export cli=../cmd/cli/cli

if [ ! -f $cli ]; then
	echo "cli command not found in ../cmd/cli, cd to ../cmd/cli and run go build"
	exit 0
fi

# call cli account generate
#
ID=`./cli_create_id.sh $1`

echo $ID

# see if we got an id, if not, exit

if [ -z $ID ]; then
   echo "Account creation failed"
   exit 0
fi

# call cli faucet 

TxID=`./cli_faucet.sh $ID $1`

# get our balance

bal=`./cli_get_balance.sh $ID $1`

echo $bal

# generate a key

Key=`./cli_key_generate t21key $1`

echo $key

# create account

echo "./cli_adi_create_account.sh $ID acc://t21acct t21key $1"


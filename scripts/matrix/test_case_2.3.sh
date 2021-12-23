#!/bin/bash
#
# test case 2.3
#
# create an adi with an invalid UTF-8 in the name sponsored by a lite account
# server IP:Port needed unless defaulting to localhost
#
# set cli command and see if it exists
#
export cli=../../cmd/accumulate/accumulate

if [ ! -f $cli ]; then
	echo "cli command not found in ../../cmd/cli, attempting to build"
        ./build_cli.sh
        if [ ! -f $cli ]; then
           echo "cli command failed to build"
           exit 1
        fi
fi

# call cli account generate
#
ID=`./cli_create_id.sh $1`

if [ $? -ne 0 ]; then
	echo "cli create id failed"
	exit 1
fi

echo $ID

# call cli faucet 

TxID=`./cli_faucet.sh $ID $1`

if [ $? -ne 0 ]; then
	echo "cli faucet failed"
	exit 1
fi

sleep 2.5

# get our balance

bal=`./cli_get_balance.sh $ID $1`

if [ $? -ne 0 ]; then
	echo "cli get balance failed"
	exit 1
fi

echo $bal

# generate a key

Key=`./cli_key_generate.sh t23key $1`

if [ $? -ne 0 ]; then
	echo "cli key generate failed"
	exit 1
fi

echo $key

# create account should fail

./cli_adi_create_account.sh $ID acc://t23\0xc0acct t23key $1

if [ $? -eq 0 ]; then
	echo "cli adi create account passed and it should have failed"
	exit 1
fi
exit 0

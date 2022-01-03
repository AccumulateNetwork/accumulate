#!/bin/bash
#
# test case 2.7
#
# create an adi with same encoding as lite account
# server IP:Port needed unless defaulting to localhost
#
# set cli command and see if it exists
#
export cli=../../cmd/accumulate/accumulate

if [ ! -f $cli ]; then
        echo "accumulate command not found in ../../cmd/cli, attempting to build"
        ./build_cli.sh
        if [ ! -f $cli ]; then
           echo "accumulate command failed to build"
           exit 1
        fi
fi

# call cli account generate
#
ID=`./cli_create_id.sh $1`

if [ $? -ne 0 ]; then
	echo "accumulate create id failed"
	exit 1
fi
echo $ID

# call cli faucet 

TxID=`./cli_faucet.sh $ID $1`

if [ $? -ne 0 ]; then
	echo "accumulate faucet failed"
	exit 1
fi

sleep 2.5

# get our balance

bal=`./cli_get_balance.sh $ID $1`

if [ $? -ne 0 ]; then
	echo "accumulate get balance failed"
	exit 1
fi
echo $bal

# generate a key

Key=`./cli_key_generate.sh t27key $1`

if [ $? -ne 0 ]; then
	echo "accumulate key generate failed"
	exit 1
fi
echo $key

# create account - extract acc://(48 chars) from ID as ID2
# this removes /ACME

ID2=${ID%/A*}

./cli_adi_create_account.sh $ID $ID2 t27key $1
if [ $? -eq 0 ]; then
	echo "accumulate adi create account passed and it should have failed"
	exit 1
fi

exit 0

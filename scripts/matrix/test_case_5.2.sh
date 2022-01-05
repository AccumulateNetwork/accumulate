#!/bin/bash
#
# test case 5.2
#
# fund an adi "redwagon" sponsored by a lite account
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
# get our balance

bal=`./cli_get_balance.sh $ID $1`

if [ $? -ne 0 ]; then
	echo "cli get balance failed"
	exit 1
fi
echo $bal

# generate a key

Key=`./cli_key_generate.sh t52key $1`

if [ $? -ne 0 ]; then
	echo "cli key generate failed"
	exit 1
fi
echo $key

# create account

echo `./cli_adi_create_account.sh $ID acc://t52acct t52key $1`

if [ $? -ne 0 ]; then
	echo "cli adi create account failed"
	exit 1
fi
# call cli faucet 

TxID=`./cli_faucet.sh acc://t52acct $1`

if [ $? -ne 0 ]; then
	echo "cli faucet failed"
	exit 1
fi
echo $TxID
exit 0

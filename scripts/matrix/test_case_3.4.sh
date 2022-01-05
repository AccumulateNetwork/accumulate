#!/bin/bash
#
# test case 3.4
#
# fund an adi "redwagon" sponsored by a lite account and get the balance
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
sleep 2.5
bal=`./cli_get_balance.sh $ID $1`

if [ $? -ne 0 ]; then
	echo "cli get balance failed"
	exit 1
fi
echo $bal

# generate a key

Key=`./cli_key_generate.sh t34key $1`

if [ $? -ne 0 ]; then
	echo "cli key generate failed"
	exit 1
fi
echo $key

# create account

sleep 2.5
echo `./cli_adi_create_account.sh $ID acc://t34acct t34key $1`

if [ $? -ne 0 ]; then
	echo "cli adi create account failed"
	exit 1
fi
# call cli faucet 

sleep 2.5
TxID=`./cli_faucet.sh acc://t34acct $1`
if [ $? -ne 0 ]; then
	echo "cli faucet failed"
	exit 1
fi

# get our balance

sleep 2.5
bal=`./cli_get_balance.sh acc://t34acct $1`
if [ $? -ne 0 ]; then
	echo "cli get balance failed"
	exit 1
fi

echo $bal
exit 0

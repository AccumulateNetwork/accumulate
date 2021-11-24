#!/bin/bash
#
# test case 3.4
#
# fund an adi "redwagon" sponsored by a lite account and get the balance
# server IP:Port needed unless defaulting to localhost
#
# set cli command and see if it exists
#
export cli=../cmd/cli/cli

if [ ! -f $cli ]; then
        echo "cli command not found in ../cmd/cli, attempting to build"
        ./build_cli.sh
        if [ ! -f $cli ]; then
                echo "cli command failed to build"
                exit 0
        fi
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

Key=`./cli_key_generate.sh t34key $1`

echo $key

# create account

echo `./cli_adi_create_account.sh $ID acc://t34acct t34key $1`

# call cli faucet 

TxID=`./cli_faucet.sh acc://t34acct $1`

# get our balance

bal=`./cli_get_balance.sh acc://t34acct $1`

echo $bal

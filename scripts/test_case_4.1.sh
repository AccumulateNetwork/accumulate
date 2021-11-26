#!/bin/bash 
#
# test case 4.1
#
# create an adi unbound key page
# server IP:Port needed unless defaulting to localhost
#
# set cli command and see if it exists
#
export cli=../cmd/cli/cli

if [ -z $1 ]; then
	echo "must supply host:port"
	exit 0
fi

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

sleep .5

# call cli faucet 

TxID=`./cli_faucet.sh $ID $1`

# generate a key

Key=`./cli_key_generate.sh t41key $1`
Key2=`./cli_key_generate.sh t41key2 $1`

#echo $Key
#echo $Key2

# create account

sleep 2
./cli_adi_create_account.sh $ID acc://t41acct t41key $1

sleep 2
$cli page create acc://t41acct t41key acc://t41acct/keypage41 t41key2 -s http://$1/v1


#!/bin/bash 
#
# test case 4.2
#
# create an adi unbound key page and create a key book from it
# server IP:Port needed unless defaulting to localhost
#
# set cli command and see if it exists
#
export cli=../../cmd/accumulate/accumulate

if [ -z $1 ]; then
	echo "must supply host:port"
	exit 1
fi

if [ ! -f $cli ]; then
	echo "accumulate command not found in ../../cmd/accumulate, attempting to build"
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

sleep .5

# call cli faucet 

TxID=`./cli_faucet.sh $ID $1`
if [ $? -ne 0 ]; then
	echo "accumulate faucet failed"
	exit 1
fi

# generate a key

Key=`./cli_key_generate.sh t42key $1`
if [ $? -ne 0 ]; then
	echo "accumulate key generate failed"
	exit 1
fi
Key2=`./cli_key_generate.sh t42key2 $1`
if [ $? -ne 0 ]; then
	echo "accumulate key generate failed"
	exit 1
fi

# create account

sleep 2
./cli_adi_create_account.sh $ID acc://t42acct t42key $1
if [ $? -ne 0 ]; then
	echo "accumulate adi create account failed"
	exit 1
fi

sleep 2
$cli page create acc://t42acct t42key acc://t42acct/keypage42 t42key2 -s http://$1/v1
if [ $? -ne 0 ]; then
	echo "accumulate page create failed"
	exit 1
fi

sleep 2

$cli book create acc://t42acct t42key acc://t42acct/book42 acc://t42acct/keypage42 -s http://$1/v1
if [ $? -ne 0 ]; then
	echo "accumulate book create failed"
	exit 1
fi


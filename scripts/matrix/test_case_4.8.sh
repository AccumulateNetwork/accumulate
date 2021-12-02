#!/bin/bash 
#
# test case 4.8
#
# sign a transaction with a secondary key page
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
ID2=`./cli_create_id.sh $1`

echo $ID
echo $ID2

# see if we got an id, if not, exit

if [ -z $ID ]; then
   echo "Account creation failed"
   exit 0
fi

sleep .5

# call cli faucet 

TxID=`./cli_faucet.sh $ID $1`

sleep 2
TxID=`./cli_faucet.sh $ID2 $1`

# generate a key

Key=`./cli_key_generate.sh t48key $1`
Key2=`./cli_key_generate.sh t48key2 $1`
Key3=`./cli_key_generate.sh t48key3 $1`
Key4=`./cli_key_generate.sh t48key4 $1`

#echo $Key
#echo $Key2

# create account

sleep 2
./cli_adi_create_account.sh $ID acc://t48acct t48key $1

sleep 2
./cli_adi_create_account.sh $ID2 acc://t48acct2 t48key2 $1

sleep 2
$cli page create acc://t48acct t48key acc://t48acct/keypage48 t48key3 -s http://$1/v1

sleep 2
$cli page create acc://t48acct t48key acc://t48acct/keypage48_2 t48key4 -s http://$1/v1

sleep 2.5
$cli book create acc://t48acct t48key acc://t48acct/book48 acc://t48acct/keypage48 acc://t48acct/keypage48_2 -s http://$1/v1

sleep 2.5
$cli account create acc://t48acct t48key acc://t48acct/myacmeacct acc://ACME acc://t48acct/book48 -s http://$1/v1 -j
sleep 2.5
$cli account create acc://t48acct2 t48key2 acc://t48acct2/myacmeacct2 acc://ACME acc://t48acct2/ssg0 -s http://$1/v1 -j

sleep 2.5
$cli tx create $ID acc://t48acct/myacmeacct 10 -s http://$1/v1 -j

sleep 2.5
$cli tx create acc://t48acct/myacmeacct t48key4 1 1 acc://t48acct2/myacmeacct2 10 -s http://$1/v1 -j



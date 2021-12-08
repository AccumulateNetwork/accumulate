#!/bin/bash 
#
# test case 4.5
#
# create an adi account bound to a key book
# server IP:Port needed unless defaulting to localhost
#
# set cli command and see if it exists
#
export cli=../../cmd/cli/cli

if [ -z $1 ]; then
	echo "must supply host:port"
	exit 1
fi

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

sleep .5

# call cli faucet 

TxID=`./cli_faucet.sh $ID $1`
if [ $? -ne 0 ]; then
	echo "cli faucet failed"
	exit 1
fi

# generate a key

Key=`./cli_key_generate.sh t45key $1`
if [ $? -ne 0 ]; then
	echo "cli key generate failed"
	exit 1
fi
Key2=`./cli_key_generate.sh t45key2 $1`
if [ $? -ne 0 ]; then
	echo "cli key generate failed"
	exit 1
fi
Key3=`./cli_key_generate.sh t45key3 $1`
if [ $? -ne 0 ]; then
	echo "cli key generate failed"
	exit 1
fi

# create account

sleep 2
./cli_adi_create_account.sh $ID acc://t45acct t45key $1
if [ $? -ne 0 ]; then
	echo "cli adi create account failed"
	exit 1
fi

sleep 2
$cli page create acc://t45acct t45key acc://t45acct/keypage45 t45key2 -s http://$1/v1
if [ $? -ne 0 ]; then
	echo "cli page create failed"
	exit 1
fi

sleep 2
$cli book create acc://t45acct t45key acc://t45acct/book45 acc://t45acct/keypage45 -s http://$1/v1
if [ $? -ne 0 ]; then
	echo "cli book create failed"
	exit 1
fi

sleep 2
$cli adi create $ID acc://t45acct2 t45key3 acc://t45acct/book45 acc://t45acct/keypage45 -s http://$1/v1
if [ $? -ne 0 ]; then
	echo "cli adi create failed"
	exit 1
fi

exit 0

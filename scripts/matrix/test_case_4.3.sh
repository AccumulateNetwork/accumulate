#!/bin/bash 
#
# test case 4.3
#
# create an adi unbound key page and add a key to it
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

Key=`./cli_key_generate.sh t43key $1`
if [ $? -ne 0 ]; then
	echo "cli key generate failed"
	exit 1
fi
Key2=`./cli_key_generate.sh t43key2 $1`
if [ $? -ne 0 ]; then
	echo "cli key generate failed"
	exit 1
fi
Key3=`./cli_key_generate.sh t43key3 $1`
if [ $? -ne 0 ]; then
	echo "cli key generate failed"
	exit 1
fi

# create account

sleep 2
./cli_adi_create_account.sh $ID acc://t43acct t43key $1
if [ $? -ne 0 ]; then
	echo "cli adi create account failed"
	exit 1
fi

sleep 2
$cli page create acc://t43acct t43key acc://t43acct/keypage43 t43key2 -s http://$1/v1
if [ $? -ne 0 ]; then
	echo "cli page create failed"
	exit 1
fi

sleep 2
$cli page key add acc://t43acct/keypage43 t43key t43key3 -s http://$1/v1

if [ $? -ne 0 ]; then
	echo "cli page key add failed"
	exit 1
fi
exit 0

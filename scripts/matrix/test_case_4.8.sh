#!/bin/bash 
#
# test case 4.8
#
# sign a transaction with a secondary key page
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
ID2=`./cli_create_id.sh $1`
if [ $? -ne 0 ]; then
	echo "cli create id failed"
	exit 1
fi

echo $ID
echo $ID2

sleep .5

# call cli faucet 

TxID=`./cli_faucet.sh $ID $1`
if [ $? -ne 0 ]; then
	echo "cli faucet failed"
	exit 1
fi

sleep 2
TxID=`./cli_faucet.sh $ID2 $1`
if [ $? -ne 0 ]; then
	echo "cli faucet failed"
	exit 1
fi

# generate a key

Key=`./cli_key_generate.sh t48key $1`
if [ $? -ne 0 ]; then
	echo "cli key generate failed"
	exit 1
fi
Key2=`./cli_key_generate.sh t48key2 $1`
if [ $? -ne 0 ]; then
	echo "cli key generate failed"
	exit 1
fi
Key3=`./cli_key_generate.sh t48key3 $1`
if [ $? -ne 0 ]; then
	echo "cli key generate failed"
	exit 1
fi
Key4=`./cli_key_generate.sh t48key4 $1`
if [ $? -ne 0 ]; then
	echo "cli key generate failed"
	exit 1
fi

# create account

sleep 2
./cli_adi_create_account.sh $ID acc://t48acct t48key $1
if [ $? -ne 0 ]; then
	echo "cli adi create account failed"
	exit 1
fi

sleep 2
./cli_adi_create_account.sh $ID2 acc://t48acct2 t48key2 $1
if [ $? -ne 0 ]; then
	echo "cli adi create account failed"
	exit 1
fi

sleep 2
$cli page create acc://t48acct t48key acc://t48acct/keypage48 t48key3 -s http://$1/v1
if [ $? -ne 0 ]; then
	echo "cli page create failed"
	exit 1
fi

sleep 2
$cli page create acc://t48acct t48key acc://t48acct/keypage48_2 t48key4 -s http://$1/v1
if [ $? -ne 0 ]; then
	echo "cli page create failed"
	exit 1
fi

sleep 2.5
$cli book create acc://t48acct t48key acc://t48acct/book48 acc://t48acct/keypage48 acc://t48acct/keypage48_2 -s http://$1/v1
if [ $? -ne 0 ]; then
	echo "cli book create failed"
	exit 1
fi

sleep 2.5
$cli account create token acc://t48acct t48key acc://t48acct/myacmeacct acc://ACME acc://t48acct/book48 -s http://$1/v1 -j
if [ $? -ne 0 ]; then
	echo "cli account create failed"
	exit 1
fi
sleep 2.5
$cli account create token acc://t48acct2 t48key2 acc://t48acct2/myacmeacct2 acc://ACME acc://t48acct2/ssg0 -s http://$1/v1 -j
if [ $? -ne 0 ]; then
	echo "cli account create failed"
	exit 1
fi

sleep 2.5
$cli tx create $ID acc://t48acct/myacmeacct 10 -s http://$1/v1 -j
if [ $? -ne 0 ]; then
	echo "cli tx create failed"
	exit 1
fi

sleep 2.5
$cli tx create acc://t48acct/myacmeacct t48key4 1 1 acc://t48acct2/myacmeacct2 10 -s http://$1/v1 -j
if [ $? -ne 0 ]; then
	echo "cli tx create failed"
	exit 1
fi
exit 0


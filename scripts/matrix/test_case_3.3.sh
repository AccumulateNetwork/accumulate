#!/bin/bash
#
# test case 3.3
#
# create an adi token account with an invalid URL
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
# get our balance
sleep 2.5
bal=`./cli_get_balance.sh $ID $1`

if [ $? -ne 0 ]; then
	echo "accumulate get balance failed"
	exit 1
fi
echo $bal

# generate a key

Key=`./cli_key_generate.sh t33key $1`

if [ $? -ne 0 ]; then
	echo "accumulate key generate failed"
	exit 1
fi
echo $key

# create account

./cli_adi_create_account.sh $ID acc://t33acct t33key $1

if [ $? -ne 0 ]; then
	echo "accumulate adi create account failed"
	exit 1
fi

# create account with invalid URL

sleep 2.5
$cli account create token acc://t33acct t33key acc:://t33acct/myacmeacct acc://ACME acc://t33acct/book0 -s http://$1/v1
if [ $? -eq 0 ]; then
	echo "accumulate account create passed and it should have failed"
	exit 1
fi
exit 0


#!/bin/bash

#accumulate wallet serve

#section "Get Version"
RESULT=$(curl  -X GET --data-binary '{"jsonrpc": "2.0", "id": 0, "method":"version", "params":{}}'  -H 'content-type:text/plain;' http://localhost:26661/wallet)
echo $RESULT


RESULT=$(curl  -X GET --data-binary '{"jsonrpc": "2.0", "id": 0, "method":"adi-list", "params":{}}'  -H 'content-type:text/plain;' http://localhost:26661/wallet)
echo $RESULT

#curl  -X GET --data-binary '{"jsonrpc": "2.0", "id": 0, "method":"key-list", "params":{}}'  -H 'content-type:text/plain;' http://localhost:26661/wallet


#curl  -X GET --data-binary '{"jsonrpc": "2.0", "id": 0, "method":"new-transaction", "params":{"txName":"test-tx1","origin":"acc//test"}}' -H  'content-type:text/plain;' http://localhost:26661/wallet
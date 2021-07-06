#!/bin/sh
#!/bin/sh

num=0
one=1

script='echo {\"jsonrpc\": \"2.0\", \"id\": $1, \"method\": \"factoid-submit\", \"params\":{\"transaction\":\"0201565d109233010100b0a0e100646f3e8750c550e4582eca5047546ffef89c13a175985e320232bacac81cc428afd7c200ce7b98bfdae90f942bc1fe88c3dd44d8f4c81f4eeb88a5602da05abc82ffdb5301718b5edd2914acc2e4677f336c1a32736e5e9bde13663e6413894f57ec272e28dc1908f98b79df30005a99df3c5caf362722e56eb0e394d20d61d34ff66c079afad1d09eee21dcd4ddaafbb65aacea4d5c1afcd086377d77172f15b3aa32250a\"}}'

for num in `seq 1 1`; do 
    payload="'`/bin/sh -c "$script" -- "$num"`'"
    #curlit="curl -X POST -H 'content-type:text/plain;' http://192.168.0.102:1234/v2 --data-binary $payload"
    curlit="curl -X POST -H 'content-type:text/plain;' http://localhost:1234/v2 --data-binary $payload"
    /bin/sh -c "$curlit"  & 
    num=$(($num+$one))
done
  
 
  

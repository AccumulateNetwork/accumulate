rm -rf  C:/Users/jathi/.accumulate
ACC_API=http://127.0.1.1:26660/v2
echo $ACC_API
go build ./cmd/accumulate
echo "generating new lite token accounts......."
./accumulate account generate
./accumulate account generate
accounts=($(./accumulate account list  | grep acc://))
export Account1=${accounts[0]}
export Account2=${accounts[1]}
echo "funding your new accounts......"
for var in 1 2 3 4 5 6 7 8 9 10
do 
echo $(./accumulate faucet $Account1 -s local)
echo $(./accumulate faucet $Account2 -s local)

done
echo "accounts created  $Account1 $Account2 happy accumulating! :)"

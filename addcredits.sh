ACC_API=http://127.0.1.1:26660/v2
accounts=($(./accumulate account list  | grep acc://))
export Account1=${accounts[0]}
export Account2=${accounts[1]}
echo "Buying credits for the following accounts ......"
echo $Account1 $Account2
./accumulate credits $Account1 $Account1 10000 -s local
./accumulate credits $Account2 $Account2 10000 -s local

./accumulate tx create $Account1 $Account2 50 -s local

echo "accounts credited  :>  $Account1 $Account2"

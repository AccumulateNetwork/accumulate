ACC_API=http://127.0.1.1:26660/v2
accounts=($(./accumulate account list  | grep acc://))
export Account1=${accounts[0]}
export Account2=${accounts[1]}
echo "Buying credits for your ADI "
./accumulate credits $Account1 acc://testadi1/testbook1/1 5000 -s local

#./accumulate credits $Account1 acc://testadi1/testtokenacc1 5000 -s local
echo "Credits added :>"
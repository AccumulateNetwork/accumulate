ACC_API=http://127.0.1.1:26660/v2
accounts=($(./accumulate account list  | grep acc://))
export Account1=${accounts[0]}
export Account2=${accounts[1]}
#echo $Account1 $Account2
echo "*** Generating new Keys for signing ***"
./accumulate key generate testkey1
echo "Creating a new ADI ..."
./accumulate adi create $Account1 testadi1 testkey1 testbook1 testpage1 -s local
echo "ADI Created!!"
echo "Buying credits for your ADI "
./accumulate credits $Account1 acc://testadi1/testpage1 1000 -s local
echo "Credits added :>"
#echo "your adi is $(./accumulate adi get acc://testadi1 -s local)"

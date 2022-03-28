ACC_API=http://127.0.1.1:26660/v2
accounts=($(./accumulate account list  | grep acc://))
export Account1=${accounts[0]}
export Account2=${accounts[1]}

echo "Creating ADI token account......"
./accumulate account create token testadi1 testkey1 testadi1/token acc://ACME acc://testadi1/testbook1 -s local
#./accumulate credits $Account1 acc://testadi1/token 1000 -s local
echo "Done."
echo "Creating ADI data account..."
./accumulate account create data testadi1 testkey1 testadi1/data acc://testadi1/testbook1 -s local
echo "Done."
echo "Creating data lite account...."
litedata=$(./accumulate.exe account create data lite $Account1 k01 k45 -s local |  awk '/Account Url/{print $3}'  | cut -c 2-)
# >> litdata.txt
echo "Done."
#export $litedata=$(cat litdata.txt | cut -c 1)

echo "Adding testdata to adi data account ...."
./accumulate data write acc://testadi1/data testkey1 "k01" "k02" "k03" "testdata" -s local 
echo "Done."

echo "Writing sample data to lite data accounts...."
echo $litedata
./accumulate data write-to $Account1  $litedata testkey1 "k01" "k02" "k055" "testdata"  -s local
echo "Done."
#accountcreation.sh
#devnetdeploy.sh
#addcredits.sh
#adicreation.sh
#datacc.sh

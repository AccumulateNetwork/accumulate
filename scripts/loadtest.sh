#./accumulated init dual beta.bvn0 tcp://bvn0.beta.testnet.accumulatenetwork.io:16591 -w .nodes
NETWORK_URL=http://127.0.1.1:26756

./accumulated.exe init dual $NETWORK_URL -w .nodes1 -l http://127.0.1.1:36656
./accumulated.exe init dual $NETWORK_URL -w .nodes2  -l http://127.0.1.1:36756
./accumulated.exe init dual $NETWORK_URL -w .nodes3  -l http://127.0.1.1:36856
./accumulated.exe init node http://127.0.1.1:26756 -l http://127.0.0.1:3656 -w .nodes1
./accumulated.exe init dual tcp://bvn0.beta.testnet.accumulatenetwork.io:16691 -l http://127.0.0.1:3656 -w .nodes4 --skip-version-check
./accumulated.exe run-dual ./.nodes4/dnn ./.nodes4/bvnn -w .nodes4

./accumulated.exe run -w ./.nodes1/bvnn
#./accumulated.exe run-dual ./.nodes1/dnn ./.nodes1/bvnn -w .nodes1
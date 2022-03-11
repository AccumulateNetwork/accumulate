echo "Cleaning Directories and deploying a new Devnet....."
go build ./cmd/accumulated
rm -rf .nodes 
mkdir .nodes
go run ./cmd/accumulated init devnet -w .nodes -f 0 -v 1 --no-empty-blocks 
#go run ./cmd/accumulated init devnet -w .nodes  --no-empty-blocks 

go run ./cmd/accumulated run devnet -w .nodes

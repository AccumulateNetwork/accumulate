package database

//go:generate go run ../../tools/cmd/gen-record --package database records.yml
//-go:generate go run ../../tools/cmd/gen-record --language yaml --out types_gen.yml records.yml -x Chain,ChangeSet,AccountData
//go:generate go run ../../tools/cmd/gen-types --package database types.yml v1.yml

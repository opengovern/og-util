currentDir=$(pwd)
cd proto || exit 1
protoc --go_out=src/golang --go_opt=paths=source_relative \
    --go-grpc_out=src/golang --go-grpc_opt=paths=source_relative \
    ./*.proto
#mv src/golang/proto/* proto/src/golang/
#rm -rf src/golang/proto
cd "$currentDir" || exit 1
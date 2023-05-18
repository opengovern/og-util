protoc --go_out=proto/src/golang --go_opt=paths=source_relative \
    --go-grpc_out=proto/src/golang --go-grpc_opt=paths=source_relative \
    proto/*.proto
mv proto/src/golang/proto/* proto/src/golang/
rm -rf proto/src/golang/proto
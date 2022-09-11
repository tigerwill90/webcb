protoc --go_out=. \
       --go_opt=paths=source_relative \
       --go-grpc_out=. \
       --go-grpc_opt=paths=source_relative \
       --go-vtproto_out=. --plugin protoc-gen-go-vtproto="${GOPATH}/bin/protoc-gen-go-vtproto"  \
       --go-vtproto_opt=features=marshal+unmarshal+size \
       --go-vtproto_opt=paths=source_relative \
       proto/cp.proto
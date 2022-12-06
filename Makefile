.PHONY: protoc_gen
protoc_gen:
	@echo "--- protoc generate start ---"
	@protoc \
		--proto_path=. \
		--go_out=. \
		--go_opt=module=github.com/go-leo/grpcproxy \
		--go-grpc_out=. \
		--go-grpc_opt=module=github.com/go-leo/grpcproxy \
		--go-grpcproxy_out=. \
		--go-grpcproxy_opt=module=github.com/go-leo/grpcproxy \
		example/api/*/*.proto
	@echo "--- protoc generate end ---"
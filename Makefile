SCHEMA_DIR := ./schema
GENERATED_DIR := ./generated
GENERATED_CHAT_DIR := $(GENERATED_DIR)/chat

protos:
	mkdir -pv $(GENERATED_CHAT_DIR)
	protoc \
		-I ${SCHEMA_DIR} \
		-I ${GOPATH}/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis \
		-I .:${GOPATH}/src:${GOPATH}/src/github.com/gogo/protobuf/protobuf \
		--gogofast_out=plugins=grpc:${GENERATED_CHAT_DIR} \
		--grpc-gateway_out=${GENERATED_CHAT_DIR} ${SCHEMA_DIR}/chat.proto \
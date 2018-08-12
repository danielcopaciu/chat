# get name of directory containing this Makefile
# (stolen from https://stackoverflow.com/a/18137056)
mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
base_dir := $(notdir $(patsubst %/,%,$(dir $(mkfile_path))))

SERVICE ?= $(base_dir)

DOCKER_ID=nesze
DOCKER_REPOSITORY_IMAGE=$(SERVICE)
DOCKER_REGISTRY=docker.io
DOCKER_REPOSITORY=$(DOCKER_REGISTRY)/$(DOCKER_ID)/$(DOCKER_REPOSITORY_IMAGE)

BUILDENV += CGO_ENABLED=0
CIRCLE_BUILD_NUM := $(CIRCLE_BUILD_NUM)
GIT_HASH := $(CIRCLE_SHA1)
ifeq ($(GIT_HASH),)
  GIT_HASH := $(shell git rev-parse HEAD)
endif
LINKFLAGS :=-s -X main.gitHash=$(GIT_HASH) -extldflags "-static"
TESTFLAGS := -v -cover
LINT_FLAGS :=--disable-all 
LINTER_EXE := gometalinter.v1
LINTER := $(GOPATH)/bin/$(LINTER_EXE)

EMPTY :=
SPACE := $(EMPTY) $(EMPTY)
join-with = $(subst $(SPACE),$1,$(strip $2))

LEXC :=
ifdef LINT_EXCLUDE
	LEXC := $(call join-with,|,$(LINT_EXCLUDE))
endif

.PHONY: install
install:
	go get -v -t -d ./...

$(LINTER):
	go get -u gopkg.in/alecthomas/$(LINTER_EXE)
	$(LINTER) --install

.PHONY: lint
lint: $(LINTER)
ifdef LEXC
	$(LINTER) --exclude '$(LEXC)' $(LINT_FLAGS) ./...
else
	$(LINTER) $(LINT_FLAGS) ./...
endif

.PHONY: clean
clean:
	rm -f $(SERVICE)

# builds our binary
$(SERVICE):
	$(BUILDENV) go build -o $(SERVICE)  -ldflags '$(LINKFLAGS)'

build: $(SERVICE)

.PHONY: test
test:
	$(BUILDENV) go test $(TESTFLAGS) ./...

.PHONY: all
all: clean $(LINTER) lint test build

docker-image:
	docker build --no-cache -t $(DOCKER_REPOSITORY):local . --build-arg SERVICE=$(SERVICE)

ci-docker-auth:
	@echo "Logging in to $(DOCKER_REGISTRY) as $(DOCKER_ID)"
	@docker login -u $(DOCKER_ID) -p $(DOCKER_PASSWORD) $(DOCKER_REGISTRY)

ci-docker-build: ci-docker-auth
	docker build --no-cache -t $(DOCKER_REPOSITORY):$(GIT_HASH)$(CIRCLE_BUILD_NUM) . --build-arg SERVICE=$(SERVICE) --build-arg GITHUB_TOKEN=$(GITHUB_TOKEN)
	docker tag $(DOCKER_REPOSITORY):$(GIT_HASH)$(CIRCLE_BUILD_NUM) $(DOCKER_REPOSITORY):latest
	docker push $(DOCKER_REPOSITORY):$(GIT_HASH)$(CIRCLE_BUILD_NUM)
	docker push $(DOCKER_REPOSITORY):latest

TARGET_USER:=$(TARGET_USER)
TARGET_SERVER:=$(TARGET_SERVER)

ci-deploy:
	ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no $(TARGET_USER)@$(TARGET_SERVER) 'docker stop $(SERVICE) || true && docker rm $(SERVICE) || true && docker pull $(DOCKER_REPOSITORY):latest && docker run -d --restart unless-stopped --name $(SERVICE) -p 8080:8080 $(DOCKER_REPOSITORY):latest'
       
        
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

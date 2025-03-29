CURR_DIR := $(shell pwd)
ARCH := $(shell uname -m)

ENTRY_PORTFEL := ./azbot/cmd/azbot/*.go
BIN_PORTFEL := ./dist/bin/azbot
OPT_LIB_ENV := LD_LIBRARY_PATH=`pwd`/opt/lib/${ARCH}:${LD_LIBRARY_PATH}

default: all

help:
	@echo "make [all|clean|generate|lint|release|testall|test|run_bt|run_trade|air]"

all:
ifeq ($(ARCH), aarch64)
	@echo "build on aarch64"
endif
	@mkdir -p ./dist/bin
	@echo "build azbot"
	@go build -ldflags="-w -s" -o ${BIN_PORTFEL} ${ENTRY_PORTFEL}
	@#CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ${BIN_PORTFEL}.x86_64 ${ENTRY_PORTFEL}
	@#CGO_ENABLED=0 GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc CGO_LDFLAGS="-L ./opt/lib/aarch64" go build -o ${BIN_PORTFEL}.aarch64 ${ENTRY_PORTFEL}

clean:
	@rm -rf ./dist/bin/*

generate:
	go generate ./...

lint:
	golangci-lint run

release:
	goreleaser build --snapshot

testall:
	go test -race -cover ./...

test:
	@#cd ./azbot/download && go test -cover -v .
	@#cd ./azbot/exchange && go test -cover -v .
	@#cd ./azbot/model && go test -cover -v .
	@#cd ./azbot/order && go test -cover -v .
	@#cd ./azbot/plot && go test -cover -v .
	@#cd ./azbot/storage && go test -cover -v .
	@#cd ./azbot/tools && go test -cover -v .
	@cd ./azbot && go test -cover -v .

run_bt:
	@go run ./cmd/azbot test -config user_data/config.yml

run_trade:
	@${OPT_LIB_ENV} go run ${ENTRY_PORTFELX} trade -config user_data/config.yml

# run and auto-reload, need on virtual env
# go install github.com/cosmtrek/air@latest
air:
	air -- backtest -config user_data/config.yml

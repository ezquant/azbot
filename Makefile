DIST := ./dist

help:
	@echo "make [generate|lint|test|build|install|release|download|backtest]"

generate:
	go generate ./...

lint:
	golangci-lint run

test:
	go test -race -cover ./...

build:
	@mkdir -p ${DIST}/bin
	go build -o ${DIST}/bin/azbot azbot/cmd/azbot/azbot.go

install:
	@echo "install azbot"
	@go install ./azbot/cmd/azbot

release:
	goreleaser build --snapshot

download:
	@echo "Download candles of BTCUSDT (Last 30 days, timeframe 1D)"
	@azbot download --pair BTCUSDT --timeframe 1d --days 30 --output ./btc-1d-last.csv

backtest:
	go run examples/backtesting/backtesting.go

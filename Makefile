DIST := ./dist

build:
	@mkdir -p ${DIST}/bin
	go build -o ${DIST}/bin/azbot azbot/cmd/azbot/azbot.go

run:
	go run examples/backtesting/backtesting.go

generate:
	go generate ./...

lint:
	golangci-lint run

test:
	go test -race -cover ./...

release:
	goreleaser build --snapshot


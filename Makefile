# tick — build, test, run. `make help` lists targets.

BIN     := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: help build install test race server docker-build docker-up ios ios-test clean

help: ## list targets
	@grep -E '^[a-z-]+:.*##' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-14s %s\n", $$1, $$2}'

build: ## build tick + tickd into ./bin
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN)/tick ./cmd/tick
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN)/tickd ./cmd/tickd

install: ## install the CLI into GOBIN
	go install -trimpath -ldflags '$(LDFLAGS)' ./cmd/tick

test: ## run Go unit tests
	go test ./...

race: ## run Go unit tests with the race detector
	go test -race ./...

server: ## run the sync server locally on :8080 (TICK_TOKEN required)
	TICK_DATA_DIR=$${TICK_DATA_DIR:-/tmp/tickd-dev} go run ./cmd/tickd

docker-build: ## build the server image
	docker build -f deploy/Dockerfile -t tickd .

docker-up: ## run the server via docker compose (reads deploy/.env)
	docker compose -f deploy/docker-compose.yml up -d --build

ios: ## generate + build the iOS app for the simulator
	cd ios && xcodegen generate && xcodebuild -project Tick.xcodeproj -scheme Tick \
		-destination 'platform=iOS Simulator,name=iPhone 17 Pro' -derivedDataPath build build

ios-test: ## run the iOS unit tests in the simulator
	cd ios && xcodegen generate && xcodebuild test -project Tick.xcodeproj -scheme Tick \
		-destination 'platform=iOS Simulator,name=iPhone 17 Pro' -derivedDataPath build

clean: ## remove build products
	rm -rf $(BIN) ios/build

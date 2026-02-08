.PHONY: build test test-unit test-e2e test-coverage clean install build-all

BINARY_NAME=zrb
BUILD_DIR=build
LDFLAGS=-s -w

build:
	go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/zrb

build-linux:
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)_linux_amd64 ./cmd/zrb

build-all:
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)_linux_amd64 ./cmd/zrb
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)_linux_arm64 ./cmd/zrb
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)_darwin_amd64 ./cmd/zrb
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)_darwin_arm64 ./cmd/zrb

test: test-unit test-e2e

test-unit:
	@echo "Running unit tests..."
	@go test -v ./internal/...

test-e2e:
	@echo "Running E2E tests..."
	@go test -v ./tests/e2e/

test-coverage:
	@echo "Generating coverage report..."
	@go test -cover ./internal/... -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

clean:
	rm -rf $(BUILD_DIR)

install: build
	install -m 755 $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/

.PHONY: build build-dev test test-unit test-e2e-vm test-all test-coverage clean

BINARY_NAME=zrb
BUILD_DIR=build
LDFLAGS=-s -w

build:
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/zrb

build-dev:
	GOOS=linux go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)_dev ./cmd/zrb

test: test-unit

test-unit:
	@echo "Running unit tests..."
	@go test -v ./internal/...

test-e2e-vm: build-dev
	@echo "Running E2E VM tests..."
	@go test -v -tags e2e_vm -timeout 30m ./test/e2e/

test-all: test-unit test-e2e-vm

test-coverage:
	@echo "Generating coverage report..."
	@go test -cover ./internal/... -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

clean:
	rm -rf $(BUILD_DIR)

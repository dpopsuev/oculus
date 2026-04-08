.PHONY: test cover lint vet

# Run all tests
test:
	go test ./...

# Run tests with coverage profile
cover:
	go test -coverprofile=/tmp/oculus-coverage.out ./...
	go tool cover -func=/tmp/oculus-coverage.out | tail -1
	@echo "Full report: go tool cover -html=/tmp/oculus-coverage.out"

# Run go vet
vet:
	go vet ./...

# Run lint + vet
lint: vet
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, skipping"; \
	fi

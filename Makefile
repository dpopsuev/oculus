.PHONY: test cover lint vet

# Run all tests
test:
	go test ./...

# Run tests with coverage profile
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1
	@echo "Full report: go tool cover -html=coverage.out"

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

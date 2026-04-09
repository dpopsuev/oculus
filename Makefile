.PHONY: test cover lint vet docker-lsp test-integration bench bench-mesh

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

# Run all benchmarks
bench:
	go test -bench=. -benchmem -timeout 600s ./...

# Run mesh-specific benchmarks
bench-mesh:
	go test -bench=BenchmarkBuildMesh -benchmem -timeout 300s .
	go test -bench=BenchmarkMergeSymbolGraph -benchmem -timeout 300s .

# Build Docker image with LSP servers for integration testing
docker-lsp:
	docker build -t oculus-lsp-test lsp/testcontainer/

# Run integration tests (requires docker-lsp image)
test-integration: docker-lsp
	go test -tags integration -timeout 300s -v ./analyzer/... -run TestLSPIntegration

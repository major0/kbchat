# kbchat - Makefile
# Implements build, test, coverage, and static analysis targets

.PHONY: test coverage coverage-html coverage-func coverage-validate clean help lint static-check security-check fmt vet mod-tidy mod-verify build-check

help:
	@echo "kbchat Build Targets"
	@echo "===================="
	@echo ""
	@echo "  test              - Run all tests"
	@echo "  coverage          - Generate coverage profile"
	@echo "  coverage-html     - Generate HTML coverage report"
	@echo "  coverage-func     - Display function-level coverage"
	@echo "  coverage-validate - Validate coverage meets targets"
	@echo "  lint              - Run golangci-lint"
	@echo "  static-check      - Run comprehensive static analysis"
	@echo "  security-check    - Run security vulnerability checks"
	@echo "  fmt               - Format Go code"
	@echo "  vet               - Run go vet"
	@echo "  mod-tidy          - Tidy go.mod and go.sum"
	@echo "  mod-verify        - Verify go.mod dependencies"
	@echo "  build-check       - Verify code builds"
	@echo "  clean             - Remove generated files"

test:
	go test -v ./...

coverage:
	go test -coverprofile=coverage.out -covermode=atomic ./...

coverage-html: coverage
	go tool cover -html=coverage.out -o coverage.html

coverage-func: coverage
	go tool cover -func=coverage.out

coverage-validate: coverage
	@./scripts/validate_coverage.sh coverage.out

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --config .golangci.yml ./...; \
	else \
		echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

static-check: fmt vet mod-tidy mod-verify lint build-check
	@echo "Static analysis complete"

security-check:
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "govulncheck not found. Install with: go install golang.org/x/vuln/cmd/govulncheck@latest"; \
	fi

fmt:
	go fmt ./...

vet:
	go vet ./...

mod-tidy:
	go mod tidy

mod-verify:
	go mod verify

build-check:
	go build -v ./...

clean:
	rm -f coverage.out coverage.html

ci-coverage: coverage coverage-validate

dev-coverage: coverage coverage-func

SQLC_VERSION := v1.31.1
SQLC := $(CURDIR)/.tools/sqlc
GOLANGCI_LINT_VERSION := v2.15.0
GOLANGCI_LINT := $(CURDIR)/.tools/golangci-lint
.PHONY: tools generate vet-sql lint test integration build

tools:
	mkdir -p .tools
	GOBIN=$(CURDIR)/.tools go install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
	GOBIN=$(CURDIR)/.tools go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
lint:
	$(GOLANGCI_LINT) run ./...
generate:
	$(SQLC) generate
vet-sql:
	$(SQLC) vet
test:
	go test -race ./...
integration:
	test -n "$(TEST_DATABASE_URL)"
	go test -race -p 1 ./internal/admin ./internal/postgres ./internal/orchestration/activities ./internal/provider/simulator -count=1
build:
	go build -o bin/ ./cmd/...

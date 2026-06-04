GO ?= go
GOLANGCI_LINT ?= golangci-lint

VERSION ?= 0.1.3
BINARY ?= terraform-provider-hpe-msa
LDFLAGS ?= -X main.version=$(VERSION)

.PHONY: build lint test testacc gofmt-check

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/tf-provider-hpe-msa

lint: gofmt-check
	$(GOLANGCI_LINT) run ./...

gofmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt required:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

test:
	$(GO) test ./...

testacc:
	TF_ACC=1 $(GO) test -tags=acc ./... -count=1

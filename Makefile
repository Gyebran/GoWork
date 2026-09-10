GO ?= go

.PHONY: run build test fmt fmt-check vet check
run:
	$(GO) run ./cmd/api
build:
	$(GO) build -trimpath -o bin/gowork-api ./cmd/api
test:
	$(GO) test -race ./...
fmt:
	$(GO) fmt ./...
fmt-check:
	@test -z "$$(gofmt -l cmd internal)" || (gofmt -l cmd internal; exit 1)
vet:
	$(GO) vet ./...
check: fmt-check vet test build

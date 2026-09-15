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

.PHONY: sqlc test-integration migrate-up migrate-down db-up db-down db-grants
sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate
test-integration:
	$(GO) test -race -tags=integration -count=1 ./tests/integration/...
migrate-up:
	$(GO) run ./cmd/migrate up
migrate-down:
	$(GO) run ./cmd/migrate down
db-up:
	docker compose up -d --wait postgres
db-down:
	docker compose down
db-grants:
	docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U gowork_owner -d gowork < db/local/grants.sql
